package v1chat

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/chat/v1"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	v1agent "github.com/w-h-a/gomento/internal/service/v1_agent"
)

type v1Handler struct {
	agent *v1agent.V1Agent
}

func (h *v1Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req struct {
		SpaceId *string `json:"space_id"`
	}

	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	var sid *uuid.UUID
	if req.SpaceId != nil && len(*req.SpaceId) > 0 {
		id, err := uuid.Parse(*req.SpaceId)
		if err != nil {
			httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
			return
		}
		sid = &id
	}

	id, err := h.agent.CreateSession(r.Context(), sid)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, map[string]string{"session_id": id.String()})
}

func (h *v1Handler) Chat(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req struct {
		SessionId string            `json:"session_id"`
		Text      string            `json:"text"`
		Files     map[string]string `json:"files"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.SessionId) == 0 {
		httphandler.WrtErr(w, http.StatusBadRequest, "session_id is required")
		return
	}

	sessId, err := uuid.Parse(req.SessionId)
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Session ID")
		return
	}

	if len(req.Text) == 0 {
		httphandler.WrtErr(w, http.StatusBadRequest, "text is required")
		return
	}

	response, toolLogs, err := h.agent.Chat(r.Context(), &v1.Chat{
		SessionId: sessId,
		Text:      req.Text,
		Files:     req.Files,
	})
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, map[string]any{
		"response":  response,
		"tool_logs": toolLogs,
	})
}

func NewV1Handler(a *v1agent.V1Agent) *v1Handler {
	return &v1Handler{
		agent: a,
	}
}
