package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"cortex/backend/internal/config"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestKafkaReportOutboxToPersistedDraft(t *testing.T) {
	kafkaURL := os.Getenv("KAFKA_TEST_URL")
	appURL, adminURL := os.Getenv("DATABASE_URL"), os.Getenv("MIGRATION_DATABASE_URL")
	if kafkaURL == "" || appURL == "" || adminURL == "" {
		t.Skip("Kafka and PostgreSQL integration URLs are not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	app, err := pgxpool.New(ctx, appURL)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	db := &store.Store{Pool: app, AdminPool: admin}
	p := domain.Principal{TenantID: uuid.New(), TenantActive: true}
	name := "kafka_report_" + uuid.NewString()
	if err = admin.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'test') RETURNING id`, name, name+"@example.invalid").Scan(&p.UserID); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, p.UserID)
	if _, err = admin.Exec(ctx, `INSERT INTO tenants(id,user_id,name) VALUES($1,$2,'Kafka report test')`, p.TenantID, p.UserID); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), `DELETE FROM tenants WHERE id=$1`, p.TenantID)
	anchor := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	var noteID int32
	if err = admin.QueryRow(ctx, `INSERT INTO notes(tenant_id,created_by,updated_by,type,title,content,note_date) VALUES($1,$2,$2,'daily','source','完成项目',$3) RETURNING id`, p.TenantID, p.UserID, anchor).Scan(&noteID); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "cortex-default" {
			t.Errorf("model=%s", body.Model)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		chunk, _ := json.Marshal(map[string]any{"id": "test", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": fmt.Sprintf("完成项目 [#%d]", noteID)}}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
	}))
	defer gateway.Close()
	job, err := db.CreateReportJob(ctx, p, uuid.New(), "monthly", anchor)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), `DELETE FROM outbox_events WHERE aggregate_type='report' AND aggregate_id=$1`, job.ID.String())
	cfg := config.Config{EventBus: "kafka", KafkaRESTURL: kafkaURL, AIBaseURL: gateway.URL + "/v1", AIAPIKey: "integration-virtual-key", AIModel: "cortex-default", Environment: "test"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	workerCtx, stop := context.WithCancel(ctx)
	relayDone := make(chan struct{})
	go func() { defer close(relayDone); runOutboxRelay(workerCtx, cfg, db, logger, "report") }()
	wait := runReportJobs(workerCtx, cfg, db, logger)
	defer func() { stop(); wait(); <-relayDone }()
	for {
		current, e := db.GetReportJob(ctx, p, job.ID)
		if e != nil {
			t.Fatal(e)
		}
		if current.Status == "failed" {
			t.Fatalf("generation failed: %v", current.FailureCode)
		}
		if current.Status == "success" {
			if current.Content != fmt.Sprintf("完成项目 [#%d]", noteID) {
				t.Fatal(current.Content)
			}
			var count int
			if err = admin.QueryRow(ctx, `SELECT count(*) FROM ai_usage_records WHERE tenant_id=$1 AND request_type='background_report'`, p.TenantID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("usage=%d err=%v", count, err)
			}
			// Redelivery after success must never invoke the gateway again.
			if _, claimed, err := db.ClaimReportJob(ctx, job.ID, uuid.New()); err != nil || claimed {
				t.Fatalf("duplicate claimed=%t err=%v", claimed, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("gateway calls=%d", calls.Load())
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("Kafka report pipeline did not complete")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
