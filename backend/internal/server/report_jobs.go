package server

import (
	"net/http"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/httpx"
	"github.com/google/uuid"
)

func (s *Server) createReportJob(w http.ResponseWriter, r *http.Request) {
	if s.cfg.EventBus != "kafka" {
		httpx.WriteError(w, s.logger, apierror.New("REPORT_BACKGROUND_UNAVAILABLE", "后台报告尚未启用，可使用实时生成", 503))
		return
	}
	if s.cfg.AIAPIKey == "" {
		httpx.WriteError(w, s.logger, apierror.New("AI_NOT_CONFIGURED", "AI 未配置", 503))
		return
	}
	var req struct {
		RequestID  uuid.UUID `json:"request_id"`
		Type       string    `json:"type"`
		AnchorDate string    `json:"anchor_date"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	anchor, err := time.Parse(time.DateOnly, req.AnchorDate)
	if err != nil || req.RequestID == uuid.Nil || (req.Type != "daily" && req.Type != "weekly" && req.Type != "monthly") {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	job, err := s.reportJobs.Create(r.Context(), principalFrom(r.Context()), req.RequestID, req.Type, anchor)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	for i := range job.Sources {
		job.Sources[i].Snippet = truncateRunes(job.Sources[i].Snippet, 160)
	}
	httpx.JSON(w, http.StatusAccepted, job)
}

func (s *Server) listReportJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.reportJobs.List(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, jobs)
}

func reportJobID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		return uuid.Nil, apierror.New("REPORT_JOB_NOT_FOUND", "报告任务不存在", 404)
	}
	return id, nil
}

func (s *Server) getReportJob(w http.ResponseWriter, r *http.Request) {
	id, err := reportJobID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	job, err := s.reportJobs.Get(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	for i := range job.Sources {
		job.Sources[i].Snippet = truncateRunes(job.Sources[i].Snippet, 160)
	}
	httpx.JSON(w, http.StatusOK, job)
}

func (s *Server) cancelReportJob(w http.ResponseWriter, r *http.Request) {
	id, err := reportJobID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if err = s.reportJobs.Cancel(r.Context(), principalFrom(r.Context()), id); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) confirmReportJob(w http.ResponseWriter, r *http.Request) {
	id, err := reportJobID(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	var req struct {
		Title     string `json:"title"`
		Content   string `json:"content"`
		Overwrite bool   `json:"overwrite"`
	}
	if err = httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	p := principalFrom(r.Context())
	result, err := s.reportJobs.Confirm(r.Context(), p, id, req.Title, req.Content, req.Overwrite)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, result)
}
