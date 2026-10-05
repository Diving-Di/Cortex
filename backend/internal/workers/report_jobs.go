package workers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"cortex/backend/internal/ai"
	"cortex/backend/internal/application/reportjobs"
	"cortex/backend/internal/config"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/eventbus"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

func runReportJobs(ctx context.Context, cfg config.Config, db *store.Store, logger *slog.Logger) func() {
	var group sync.WaitGroup
	workflow := ai.Workflow{Client: &ai.EinoClient{BaseURL: cfg.AIBaseURL, APIKey: cfg.AIAPIKey, HTTPClient: &http.Client{Timeout: 15 * time.Minute}}, Model: "cortex-default"}
	worker := reportjobs.Worker{Repository: db, Generate: func(ctx context.Context, p domain.Principal, prompt string) (<-chan ai.StreamEvent, error) {
		return workflow.GenerateReport(ai.WithRequestMetadata(ctx, ai.RequestMetadata{RequestID: uuid.NewString(), RequestType: "background_report", Tenant: p.TenantID.String(), Environment: cfg.Environment}), prompt)
	}}
	group.Add(1)
	go func() {
		defer group.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for ctx.Err() == nil {
			if err := db.ReconcileReportJobs(ctx); err != nil && ctx.Err() == nil {
				logger.Error("recover report jobs", "code", "REPORT_RECOVERY_FAILED")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	// Two bounded consumers per process. PostgreSQL also caps execution across
	// all instances at four jobs globally and one job per tenant.
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			runStageConsumer(ctx, cfg.KafkaRESTURL, "cortex-report-generation-v1", "cortex.report.generate.v1", logger, func(record eventbus.Record) bool {
				if record.Value.SchemaVersion != 1 || record.Value.Type != "report.generate.requested" {
					return true
				}
				id, err := uuid.Parse(record.Value.AggregateID)
				if err != nil {
					return true
				}
				err = worker.Process(ctx, id)
				if errors.Is(err, store.ErrReportJobLeaseLost) {
					return true
				}
				if err != nil {
					logger.Error("process report job", "code", "REPORT_JOB_FAILED")
					return false
				}
				return true
			})
		}()
	}
	return group.Wait
}
