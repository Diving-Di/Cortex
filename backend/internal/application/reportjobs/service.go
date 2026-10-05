// Package reportjobs owns durable report generation. Kafka carries only job IDs;
// prompts, source validation and draft persistence remain in the business layer.
package reportjobs

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cortex/backend/internal/ai"
	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type Repository interface {
	ClaimReportJob(context.Context, uuid.UUID, uuid.UUID) (domain.Principal, bool, error)
	GetReportJob(context.Context, domain.Principal, uuid.UUID) (store.ReportJob, error)
	ValidateReportJobSources(context.Context, domain.Principal, uuid.UUID) error
	RenewReportJob(context.Context, domain.Principal, uuid.UUID, uuid.UUID) error
	FinishReportJob(context.Context, domain.Principal, uuid.UUID, uuid.UUID, string, string, ...domain.AIUsage) error
}

type Generator func(context.Context, domain.Principal, string) (<-chan ai.StreamEvent, error)
type Worker struct {
	Repository         Repository
	Generate           Generator
	Timeout, Heartbeat time.Duration
}

func (w Worker) Process(ctx context.Context, id uuid.UUID) error {
	started := time.Now()
	owner := uuid.New()
	p, claimed, err := w.Repository.ClaimReportJob(ctx, id, owner)
	if err != nil || !claimed {
		return err
	}
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	heartbeat := w.Heartbeat
	if heartbeat <= 0 {
		heartbeat = 20 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				renewCtx, stop := context.WithTimeout(runCtx, 5*time.Second)
				err := w.Repository.RenewReportJob(renewCtx, p, id, owner)
				stop()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	j, err := w.Repository.GetReportJob(runCtx, p, id)
	if err == nil {
		err = w.Repository.ValidateReportJobSources(runCtx, p, id)
	}
	var content string
	inputTokens := 0
	if err == nil {
		var material strings.Builder
		fmt.Fprintf(&material, "仅依据以下来源撰写 %s 报告，周期起始 %s。事实使用 [#笔记ID] 引用，不得虚构。总结进展、问题、变化和有依据的下一步建议。\n", j.Type, j.AnchorDate)
		for _, source := range j.Sources {
			fmt.Fprintf(&material, "[来源 #%d %s]\n%s\n\n", source.ID, source.Title, source.Snippet)
		}
		var events <-chan ai.StreamEvent
		inputTokens = max(1, len([]rune(material.String()))/4)
		events, err = w.Generate(runCtx, p, material.String())
		if err == nil {
			content, err = collect(runCtx, events)
		}
		if err == nil {
			err = validateCitations(content, j.Sources)
		}
	}
	code := ""
	if err != nil {
		code = "REPORT_GENERATION_FAILED"
		if err.Error() == "AI_NOT_CONFIGURED" {
			code = "AI_NOT_CONFIGURED"
		}
		var appErr *apierror.Error
		if errors.As(err, &appErr) {
			switch appErr.Code {
			case "REPORT_SOURCES_CHANGED", "REPORT_INVALID_CITATIONS", "AI_NOT_CONFIGURED":
				code = appErr.Code
			}
		}
		if runCtx.Err() != nil {
			code = "REPORT_GENERATION_INTERRUPTED"
		}
	}
	// The owner fence is checked again on completion. Stop heartbeat before
	// committing so a late heartbeat cannot interfere with a completed job.
	cancel()
	<-stopped
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finishCancel()
	status := "success"
	var errorCode *string
	if code != "" {
		status = "failed"
		errorCode = &code
	}
	return w.Repository.FinishReportJob(finishCtx, p, id, owner, content, code, domain.AIUsage{RequestType: "background_report", Model: "cortex-default", InputTokens: inputTokens, OutputTokens: len([]rune(content)) / 4, Duration: time.Since(started), Status: status, ErrorCode: errorCode})
}

func collect(ctx context.Context, events <-chan ai.StreamEvent) (string, error) {
	var output strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case event, ok := <-events:
			if !ok {
				if strings.TrimSpace(output.String()) == "" {
					return "", errors.New("empty report")
				}
				return output.String(), nil
			}
			if event.Err != nil {
				return "", event.Err
			}
			if output.Len()+len(event.Content) > 2*1024*1024 {
				return "", errors.New("report too large")
			}
			output.WriteString(event.Content)
		}
	}
}

var citationPattern = regexp.MustCompile(`\[#([0-9]+)\]`)

func validateCitations(content string, sources []store.SourceNote) error {
	allowed := make(map[int32]bool, len(sources))
	for _, source := range sources {
		allowed[source.ID] = true
	}
	citations := citationPattern.FindAllStringSubmatch(content, -1)
	if len(citations) == 0 {
		return apierror.New("REPORT_INVALID_CITATIONS", "报告缺少有效来源引用", 422)
	}
	for _, citation := range citations {
		id, err := strconv.ParseInt(citation[1], 10, 32)
		if err != nil || !allowed[int32(id)] {
			return apierror.New("REPORT_INVALID_CITATIONS", "报告包含无效来源引用", 422)
		}
	}
	return nil
}

func ValidateConfirmation(title, content string, sources []store.SourceNote) error {
	if strings.TrimSpace(title) == "" || len([]rune(title)) > 200 || strings.TrimSpace(content) == "" || len(content) > 2*1024*1024 {
		return apierror.Validation(nil)
	}
	return validateCitations(content, sources)
}
