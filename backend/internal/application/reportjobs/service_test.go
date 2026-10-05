package reportjobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"cortex/backend/internal/ai"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type fakeRepository struct {
	claimed       bool
	renewErr      error
	content, code string
	finishErr     error
	sourceErr     error
}

func (f *fakeRepository) ClaimReportJob(context.Context, uuid.UUID, uuid.UUID) (domain.Principal, bool, error) {
	return domain.Principal{}, f.claimed, nil
}
func (f *fakeRepository) GetReportJob(context.Context, domain.Principal, uuid.UUID) (store.ReportJob, error) {
	return store.ReportJob{Type: "monthly", Sources: []store.SourceNote{{ID: 1, Title: "工作", Snippet: "完成项目"}}}, nil
}
func (f *fakeRepository) ValidateReportJobSources(context.Context, domain.Principal, uuid.UUID) error {
	return f.sourceErr
}
func (f *fakeRepository) RenewReportJob(context.Context, domain.Principal, uuid.UUID, uuid.UUID) error {
	return f.renewErr
}
func (f *fakeRepository) FinishReportJob(_ context.Context, _ domain.Principal, _, _ uuid.UUID, content, code string, _ ...domain.AIUsage) error {
	f.content = content
	f.code = code
	return f.finishErr
}

func TestWorkerDuplicateDoesNotGenerate(t *testing.T) {
	f := &fakeRepository{}
	w := Worker{Repository: f, Generate: func(context.Context, domain.Principal, string) (<-chan ai.StreamEvent, error) {
		t.Fatal("duplicate generated")
		return nil, nil
	}}
	if err := w.Process(context.Background(), uuid.New()); err != nil {
		t.Fatal(err)
	}
}
func TestWorkerDoesNotRetryPartialGeneration(t *testing.T) {
	f := &fakeRepository{claimed: true}
	calls := 0
	w := Worker{Repository: f, Generate: func(context.Context, domain.Principal, string) (<-chan ai.StreamEvent, error) {
		calls++
		events := make(chan ai.StreamEvent, 2)
		events <- ai.StreamEvent{Content: "partial [#1]"}
		events <- ai.StreamEvent{Err: errors.New("upstream secret response")}
		close(events)
		return events, nil
	}}
	if err := w.Process(context.Background(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || f.content != "" || f.code != "REPORT_GENERATION_FAILED" {
		t.Fatalf("calls=%d content=%q code=%q", calls, f.content, f.code)
	}
}
func TestWorkerCancelsGenerationWhenLeaseLost(t *testing.T) {
	f := &fakeRepository{claimed: true, renewErr: store.ErrReportJobLeaseLost, finishErr: store.ErrReportJobLeaseLost}
	cancelled := make(chan struct{})
	w := Worker{Repository: f, Heartbeat: time.Millisecond, Timeout: time.Second, Generate: func(ctx context.Context, _ domain.Principal, _ string) (<-chan ai.StreamEvent, error) {
		events := make(chan ai.StreamEvent)
		go func() { <-ctx.Done(); close(cancelled) }()
		return events, nil
	}}
	if err := w.Process(context.Background(), uuid.New()); !errors.Is(err, store.ErrReportJobLeaseLost) {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("model was not cancelled")
	}
}
func TestWorkerPersistsOnlyCompleteCitedDraft(t *testing.T) {
	for _, content := range []string{"完成项目 [#1]", "没有引用", "引用越权 [#2]"} {
		t.Run(content, func(t *testing.T) {
			f := &fakeRepository{claimed: true}
			w := Worker{Repository: f, Generate: func(context.Context, domain.Principal, string) (<-chan ai.StreamEvent, error) {
				events := make(chan ai.StreamEvent, 1)
				events <- ai.StreamEvent{Content: content}
				close(events)
				return events, nil
			}}
			if err := w.Process(context.Background(), uuid.New()); err != nil {
				t.Fatal(err)
			}
			if content == "完成项目 [#1]" {
				if f.content != content || f.code != "" {
					t.Fatalf("%+v", f)
				}
			} else if f.code != "REPORT_INVALID_CITATIONS" {
				t.Fatal(f.code)
			}
		})
	}
}
