package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/httpx"
	"diary-listener/backend/internal/store"
)

type organizeRequest struct {
	Content string `json:"content"`
	Style   string `json:"style"`
}

type organizeConfirmRequest struct {
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Summary *string `json:"summary"`
	NoteID  *int32  `json:"note_id"`
}

type reportRequest struct {
	Type       string  `json:"type"`
	AnchorDate string  `json:"anchor_date"`
	Title      string  `json:"title,omitempty"`
	Content    string  `json:"content,omitempty"`
	SourceIDs  []int32 `json:"source_ids,omitempty"`
	Overwrite  bool    `json:"overwrite,omitempty"`
}

func (s *Server) aiWorkflow() ai.Workflow {
	return ai.Workflow{
		Client: &ai.OpenAICompatibleClient{BaseURL: s.cfg.AIBaseURL, APIKey: s.cfg.AIAPIKey},
		Model:  s.cfg.AIModel,
	}
}

func (s *Server) organizeAI(w http.ResponseWriter, r *http.Request) {
	var request organizeRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if len([]rune(request.Content)) < 1 || len([]rune(request.Content)) > 50000 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	if request.Style == "" {
		request.Style = "structured"
	}
	events, err := s.aiWorkflow().Organize(s.aiContext(r.Context(), "organize", principalFrom(r.Context())), request.Content, request.Style)
	if err != nil {
		if err.Error() == "AI_NOT_CONFIGURED" {
			httpx.WriteError(w, s.logger, apierror.New("AI_NOT_CONFIGURED", "AI 未配置，笔记功能仍可正常使用", 503))
		} else {
			httpx.WriteError(w, s.logger, err)
		}
		return
	}
	prompt := request.Style + "\n" + request.Content
	s.writeSSE(w, r, "organize", s.cfg.AIModel, prompt, events, nil)
}

func (s *Server) confirmOrganize(w http.ResponseWriter, r *http.Request) {
	var request organizeConfirmRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	if request.Title == "" {
		httpx.WriteError(w, s.logger, apierror.New("TITLE_REQUIRED", "标题不能为空", 422))
		return
	}
	result, err := s.store.ConfirmOrganize(
		r.Context(), principalFrom(r.Context()), request.NoteID,
		request.Title, request.Content, request.Summary,
	)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, result)
}

func (s *Server) previewReport(w http.ResponseWriter, r *http.Request) {
	request, anchor, err := decodeReportRequest(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	start, end, sources, err := s.store.ReportSources(
		r.Context(), principalFrom(r.Context()), request.Type, anchor,
	)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	for index := range sources {
		sources[index].Snippet = truncateRunes(sources[index].Snippet, 160)
	}
	if sources == nil {
		sources = []store.SourceNote{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"start_date": start.Format(time.DateOnly),
		"end_date":   end.Format(time.DateOnly),
		"sources":    sources,
	})
}

func (s *Server) generateReport(w http.ResponseWriter, r *http.Request) {
	request, anchor, err := decodeReportRequest(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	start, end, sources, err := s.store.ReportSources(
		r.Context(), principalFrom(r.Context()), request.Type, anchor,
	)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if len(sources) == 0 {
		httpx.WriteError(w, s.logger, apierror.New("REPORT_NO_SOURCES", "所选周期没有来源笔记，已阻止生成虚构报告", 422))
		return
	}
	var material strings.Builder
	for _, source := range sources {
		fmt.Fprintf(&material, "[来源 #%d %s %s]\n%s\n\n", source.ID, optionalText(source.NoteDate), source.Title, source.Snippet)
	}
	prompt := fmt.Sprintf(
		"仅依据以下来源撰写 %s Markdown 报告。每个事实使用 [#笔记ID] 引用，不得虚构。周期 %s 至 %s。\n\n%s",
		request.Type, start.Format(time.DateOnly), end.Format(time.DateOnly), material.String(),
	)
	events, err := s.aiWorkflow().GenerateReport(s.aiContext(r.Context(), "report", principalFrom(r.Context())), prompt)
	if err != nil {
		if err.Error() == "AI_NOT_CONFIGURED" {
			httpx.WriteError(w, s.logger, apierror.New("AI_NOT_CONFIGURED", "AI 未配置，笔记功能仍可正常使用", 503))
		} else {
			httpx.WriteError(w, s.logger, err)
		}
		return
	}
	s.writeSSE(w, r, "report", s.cfg.AIModel, prompt, events, nil)
}

func (s *Server) confirmReport(w http.ResponseWriter, r *http.Request) {
	request, anchor, err := decodeReportRequest(r)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.store.ConfirmReport(
		r.Context(), principalFrom(r.Context()), request.Type, anchor,
		request.Title, request.Content, request.SourceIDs, request.Overwrite,
	)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, result)
}

func (s *Server) reportSourceList(w http.ResponseWriter, r *http.Request) {
	noteID, err := pathID(r, "noteID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.store.GetReportSources(r.Context(), principalFrom(r.Context()), noteID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if result == nil {
		result = []store.SourceNote{}
	}
	httpx.JSON(w, http.StatusOK, result)
}

func decodeReportRequest(r *http.Request) (reportRequest, time.Time, error) {
	var request reportRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		return request, time.Time{}, err
	}
	if request.Type != "daily" && request.Type != "weekly" && request.Type != "monthly" {
		return request, time.Time{}, apierror.New("INVALID_REPORT_TYPE", "报告类型必须是 daily、weekly 或 monthly", 422)
	}
	anchor, err := time.Parse(time.DateOnly, request.AnchorDate)
	if err != nil {
		return request, time.Time{}, apierror.Validation(nil)
	}
	return request, anchor, nil
}

func optionalText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
