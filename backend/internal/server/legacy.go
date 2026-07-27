package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/httpx"
	"diary-listener/backend/internal/store"
	"github.com/google/uuid"
)

type legacyChatRequest struct {
	Content        string `json:"content"`
	ConversationID *int32 `json:"conversation_id"`
}

type conversationRequest struct {
	Title       string `json:"title"`
	SourceScope string `json:"source_scope"`
}

func (s *Server) listV1Conversations(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.ListScopedConversations(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if result == nil {
		result = []store.Conversation{}
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) createV1Conversation(w http.ResponseWriter, r *http.Request) {
	var request conversationRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	request.SourceScope = strings.TrimSpace(request.SourceScope)
	if request.Title == "" {
		request.Title = "新对话"
	}
	if len([]rune(request.Title)) > 80 || !store.ValidSourceScope(request.SourceScope) {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	result, err := s.store.CreateConversation(r.Context(), principalFrom(r.Context()), request.Title, request.SourceScope)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, result)
}

func (s *Server) getV1Conversation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "conversationID")
	if err == nil {
		var result store.Conversation
		result, err = s.store.GetConversation(r.Context(), principalFrom(r.Context()), id)
		if err == nil && !store.ValidSourceScope(result.SourceScope) {
			err = apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
		}
		if err == nil {
			httpx.JSON(w, http.StatusOK, result)
			return
		}
	}
	httpx.WriteError(w, s.logger, err)
}

func (s *Server) deleteV1Conversation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "conversationID")
	if err == nil {
		var result store.Conversation
		result, err = s.store.GetConversation(r.Context(), principalFrom(r.Context()), id)
		if err == nil && !store.ValidSourceScope(result.SourceScope) {
			err = apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
		}
		if err == nil {
			err = s.store.DeleteConversation(r.Context(), principalFrom(r.Context()), id)
		}
	}
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.ListConversations(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if result == nil {
		result = []store.Conversation{}
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "conversationID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result, err := s.store.GetConversation(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		writeLegacyError(w, s, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "conversationID")
	if err == nil {
		err = s.store.DeleteConversation(r.Context(), principalFrom(r.Context()), id)
	}
	if err != nil {
		writeLegacyError(w, s, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sendLegacyChat(w http.ResponseWriter, r *http.Request) {
	var request legacyChatRequest
	if err := httpx.DecodeJSON(r, &request); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	request.Content = strings.TrimSpace(request.Content)
	if request.Content == "" {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": "消息不能为空"})
		return
	}
	principal := principalFrom(r.Context())
	conversationID, title, history, err := s.store.ConversationHistory(r.Context(), principal, request.ConversationID)
	if err != nil {
		writeLegacyError(w, s, err)
		return
	}
	if conversationID == 0 {
		title = truncateRunes(request.Content, 20)
		if len([]rune(request.Content)) > 20 {
			title += "…"
		}
	}
	messages := make([]ai.Message, 0, len(history)+2)
	if s.cfg.AISystemPrompt != "" {
		messages = append(messages, ai.Message{Role: "system", Content: s.cfg.AISystemPrompt})
	}
	for _, item := range history {
		messages = append(messages, ai.Message{Role: item.Role, Content: item.Content})
	}
	messages = append(messages, ai.Message{Role: "user", Content: request.Content})
	reply := ""
	if s.cfg.AIAPIKey == "" {
		reply = "（本地模拟回复，未配置 AI API Key）\n我收到了你的消息：" + request.Content +
			"\n在 backend/config.json 中填入 DeepSeek 的 api_key 即可获得真实回复。"
	} else {
		events, err := (&ai.OpenAICompatibleClient{
			BaseURL: s.cfg.AIBaseURL, APIKey: s.cfg.AIAPIKey,
		}).StreamChat(r.Context(), ai.ChatRequest{Model: s.cfg.AIModel, Messages: messages})
		if err != nil {
			reply = "（AI 接口调用失败：" + err.Error() + "）"
		} else {
			var output strings.Builder
			for event := range events {
				if event.Err != nil {
					reply = "（AI 接口调用失败：" + event.Err.Error() + "）"
					break
				}
				output.WriteString(event.Content)
			}
			if reply == "" {
				reply = output.String()
			}
		}
	}
	result, err := s.store.SaveLegacyChat(
		r.Context(), principal, conversationID, title, request.Content, reply,
	)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) listDiary(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListDiary(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	result := make([]map[string]any, 0, len(entries))
	for _, item := range entries {
		result = append(result, serializeDiary(item))
	}
	httpx.JSON(w, http.StatusOK, result)
}

func (s *Server) createDiary(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(s.cfg.MaxAttachmentBytes + (1 << 20)); err != nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": "请求格式无效"})
		return
	}
	content := strings.TrimSpace(r.FormValue("content"))
	var storedName *string
	file, header, fileErr := r.FormFile("image")
	if fileErr == nil {
		defer file.Close()
		extension := strings.ToLower(filepath.Ext(header.Filename))
		allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true}
		if !allowed[extension] {
			httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": "不支持的图片格式"})
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, s.cfg.MaxAttachmentBytes+1))
		if err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
		if len(data) == 0 {
			httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": "图片内容为空"})
			return
		}
		if int64(len(data)) > s.cfg.MaxAttachmentBytes {
			httpx.JSON(w, http.StatusRequestEntityTooLarge, map[string]string{"detail": "图片过大"})
			return
		}
		value := strings.ReplaceAll(uuid.NewString(), "-", "")[:16] + extension
		target := filepath.Join(s.cfg.DataDir, "media", "diary", value)
		if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
		if err := os.WriteFile(target, data, 0640); err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
		storedName = &value
	}
	if content == "" && storedName == nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]string{"detail": "请上传图片或填写内容"})
		return
	}
	entry, err := s.store.CreateDiary(r.Context(), principalFrom(r.Context()), content, storedName)
	if err != nil {
		if storedName != nil {
			_ = os.Remove(filepath.Join(s.cfg.DataDir, "media", "diary", *storedName))
		}
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, serializeDiary(entry))
}

func (s *Server) deleteDiary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "entryID")
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	imagePath, err := s.store.DeleteDiary(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		writeLegacyError(w, s, err)
		return
	}
	if imagePath != nil {
		_ = os.Remove(filepath.Join(s.cfg.DataDir, "media", "diary", filepath.Base(*imagePath)))
	}
	w.WriteHeader(http.StatusNoContent)
}

func serializeDiary(item store.DiaryEntry) map[string]any {
	var image *string
	if item.ImagePath != nil {
		value := "/media/diary/" + *item.ImagePath
		image = &value
	}
	return map[string]any{
		"id": item.ID, "image": image, "content": item.Content,
		"created_at": item.CreatedAt.Format(time.RFC3339Nano),
	}
}

func writeLegacyError(w http.ResponseWriter, s *Server, err error) {
	var appErr *apierror.Error
	if errors.As(err, &appErr) &&
		(appErr.Code == "CONVERSATION_NOT_FOUND" || appErr.Code == "DIARY_NOT_FOUND") {
		httpx.JSON(w, appErr.StatusCode, map[string]string{"detail": appErr.Message})
		return
	}
	httpx.WriteError(w, s.logger, err)
}
