package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateKnowledgeFeedbackRejectsUnknownCategory(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/requests/request-12345678/feedback", bytes.NewBufferString(`{"category":"other"}`))
	r.SetPathValue("requestID", "request-12345678")
	w := httptest.NewRecorder()
	(&Server{logger: slog.Default()}).createKnowledgeFeedback(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateKnowledgeFeedbackRejectsInvalidRequestID(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/requests/x/feedback", bytes.NewBufferString(`{"category":"high_latency"}`))
	r.SetPathValue("requestID", "invalid request id")
	w := httptest.NewRecorder()
	(&Server{logger: slog.Default()}).createKnowledgeFeedback(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestPromoteKnowledgeFeedbackRejectsRawEvidence(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/feedback/1/promote", bytes.NewBufferString(`{"dataset_name":"regression","dataset_version":1,"case_id":"bad-001","query":"公开测试问题","expected_answer":"公开测试答案","evidence_hashes":["private raw text"]}`))
	r.SetPathValue("feedbackID", "1")
	w := httptest.NewRecorder()
	(&Server{logger: slog.Default()}).promoteKnowledgeFeedback(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
