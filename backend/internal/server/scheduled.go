package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/httpx"
	"diary-listener/backend/internal/store"
	"github.com/google/uuid"
	"log/slog"
)

type scheduledTaskRequest struct {
	ReportType string `json:"report_type"`
	Hour       int    `json:"hour"`
	Minute     int    `json:"minute"`
	Timezone   string `json:"timezone"`
}

func RunScheduler(
	ctx context.Context,
	cfg config.Config,
	database *store.Store,
	logger *slog.Logger,
) {
	if !cfg.ScheduledReportsEnabled {
		return
	}
	worker := &Server{cfg: cfg, store: database, logger: logger, version: "scheduler"}
	owner := uuid.New()
	ticker := time.NewTicker(cfg.ScheduledReportPoll)
	defer ticker.Stop()
	for {
		if err := worker.pollScheduledReports(ctx, owner); err != nil && ctx.Err() == nil {
			logger.Error("scheduled report poll failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) pollScheduledReports(ctx context.Context, owner uuid.UUID) error {
	claimed, err := s.store.ClaimDueScheduledTasks(ctx, owner, 10, 5*time.Minute)
	if err != nil {
		return err
	}
	for _, item := range claimed {
		principal := domain.Principal{
			UserID: item.UserID, TenantID: item.TenantID, TenantActive: true,
		}
		s.executeScheduledReport(ctx, principal, item.TaskID, "scheduled", item.LeaseOwner)
	}
	return nil
}

func (s *Server) listScheduledReports(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.ListScheduledTasks(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if result == nil {
		result = []store.ScheduledTask{}
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) createScheduledReport(w http.ResponseWriter, r *http.Request) {
	var request scheduledTaskRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if request.Timezone == "" {
		request.Timezone = "Asia/Shanghai"
	}
	if request.Hour < 0 || request.Hour > 23 || request.Minute < 0 || request.Minute > 59 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	next, err := nextScheduledRun(request.ReportType, request.Hour, request.Minute, request.Timezone, time.Now())
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.store.CreateScheduledTask(
		r.Context(), principalFrom(r.Context()), request.ReportType,
		request.Hour, request.Minute, request.Timezone, next,
	)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, result)
}

func (s *Server) setScheduledReportStatus(w http.ResponseWriter, r *http.Request) {
	taskID, err := pathID(r, "taskID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	enabled, err := strconv.ParseBool(r.URL.Query().Get("enabled"))
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	task, err := s.store.GetScheduledTask(r.Context(), principalFrom(r.Context()), taskID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	next, err := nextScheduledRun(task.ReportType, int(task.Hour), int(task.Minute), task.Timezone, time.Now())
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.store.SetScheduledTaskStatus(r.Context(), principalFrom(r.Context()), taskID, enabled, next)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) retryScheduledReport(w http.ResponseWriter, r *http.Request) {
	taskID, err := pathID(r, "taskID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	principal := principalFrom(r.Context())
	if _, err := s.store.GetScheduledTask(r.Context(), principal, taskID); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	owner := uuid.New()
	if err := s.store.AcquireScheduledTaskLease(r.Context(), principal, taskID, owner, 5*time.Minute); err != nil {
		httpx.WriteError(w, s.logger, apierror.New("SCHEDULED_REPORT_BUSY", "定时报告正在执行", 409))
		return
	}
	go s.executeScheduledReport(context.Background(), principal, taskID, "manual", owner)
	httpx.JSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (s *Server) listScheduledReportRuns(w http.ResponseWriter, r *http.Request) {
	taskID, err := pathID(r, "taskID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.store.ListScheduledRuns(r.Context(), principalFrom(r.Context()), taskID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if result == nil {
		result = []store.ScheduledRun{}
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) executeScheduledReport(ctx context.Context, principal domain.Principal, taskID int32, trigger string, owner uuid.UUID) {
	task, err := s.store.GetScheduledTask(ctx, principal, taskID)
	if err != nil {
		s.logger.Error("load scheduled report", "error", err)
		return
	}
	runID, err := s.store.StartScheduledRun(ctx, principal, taskID, trigger, owner)
	if err != nil {
		s.logger.Error("start scheduled report", "error", err)
		return
	}
	leaseCtx, stopLease := context.WithCancel(ctx)
	defer stopLease()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-ticker.C:
				ok, renewErr := s.store.RenewScheduledTaskLease(leaseCtx, principal, taskID, owner, 5*time.Minute)
				if renewErr != nil || !ok {
					if renewErr == nil || errors.Is(renewErr, store.ErrScheduledLeaseLost) {
						scheduledReportLeaseLost.Add(1)
					}
					return
				}
			}
		}
	}()
	zone, zoneErr := time.LoadLocation(task.Timezone)
	var reportNoteID *int32
	runErr := zoneErr
	if runErr == nil {
		anchor := time.Now().In(zone)
		_, _, sources, sourceErr := s.store.ReportSources(ctx, principal, task.ReportType, anchor)
		runErr = sourceErr
		if runErr == nil && len(sources) == 0 {
			runErr = apierror.New("REPORT_NO_SOURCES", "所选周期没有来源笔记", 422)
		}
		if runErr == nil && s.cfg.AIAPIKey == "" {
			runErr = apierror.New("AI_NOT_CONFIGURED", "AI 未配置", 503)
		}
		if runErr == nil {
			var material strings.Builder
			for _, source := range sources {
				fmt.Fprintf(&material, "[来源 #%d %s %s]\n%s\n\n", source.ID, optionalText(source.NoteDate), source.Title, source.Snippet)
			}
			prompt := "仅依据以下来源撰写 " + task.ReportType +
				" Markdown 报告，使用 [#笔记ID] 引用，不得虚构。\n" + material.String()
			events, streamErr := s.aiWorkflow().GenerateReport(s.aiContext(ctx, "scheduled_report", principal), prompt)
			runErr = streamErr
			var content strings.Builder
			if runErr == nil {
				for event := range events {
					if event.Err != nil {
						runErr = event.Err
						break
					}
					content.WriteString(event.Content)
				}
			}
			if runErr == nil {
				result, confirmErr := s.store.ConfirmReport(
					ctx, principal, task.ReportType, anchor,
					fmt.Sprintf("%s %s 报告", periodRangeForTitle(task.ReportType, anchor), task.ReportType),
					content.String(), sourceIDs(sources), true, &owner, taskID,
				)
				runErr = confirmErr
				if confirmErr == nil {
					value := result["id"].(int32)
					reportNoteID = &value
				}
			}
		}
	}
	next, nextErr := nextScheduledRun(task.ReportType, int(task.Hour), int(task.Minute), task.Timezone, time.Now())
	if runErr == nil {
		runErr = nextErr
	}
	if runErr != nil {
		scheduledReportRunsFailed.Add(1)
	}
	if finishErr := s.store.FinishScheduledRun(ctx, principal, task, runID, reportNoteID, runErr, next, owner); finishErr != nil {
		if errors.Is(finishErr, store.ErrScheduledLeaseLost) {
			scheduledReportLeaseLost.Add(1)
		}
		s.logger.Error("finish scheduled report", "error", finishErr)
	}
}

func nextScheduledRun(kind string, hour, minute int, timezoneName string, now time.Time) (time.Time, error) {
	zone, err := time.LoadLocation(timezoneName)
	if err != nil {
		return time.Time{}, apierror.New("INVALID_TIMEZONE", "无效的 IANA 时区", 422)
	}
	localNow := now.In(zone)
	candidate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, zone)
	switch kind {
	case "daily":
		if !candidate.After(localNow) {
			candidate = candidate.AddDate(0, 0, 1)
		}
	case "weekly":
		candidate = candidate.AddDate(0, 0, (7-int(candidate.Weekday()))%7)
		if !candidate.After(localNow) {
			candidate = candidate.AddDate(0, 0, 7)
		}
	case "monthly":
		first := time.Date(localNow.Year(), localNow.Month(), 1, hour, minute, 0, 0, zone)
		candidate = first.AddDate(0, 1, -1)
		if !candidate.After(localNow) {
			candidate = first.AddDate(0, 2, -1)
		}
	default:
		return time.Time{}, apierror.New("INVALID_REPORT_TYPE", "报告类型无效", 422)
	}
	return candidate.UTC(), nil
}

func sourceIDs(sources []store.SourceNote) []int32 {
	result := make([]int32, 0, len(sources))
	for _, source := range sources {
		result = append(result, source.ID)
	}
	return result
}

func periodRangeForTitle(kind string, anchor time.Time) string {
	start := normalizePeriod(kind, &anchor)
	return start.Format(time.DateOnly)
}
