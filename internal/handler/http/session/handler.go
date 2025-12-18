package session

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	"github.com/w-h-a/gomento/internal/service/session"
	"github.com/w-h-a/gomento/internal/util"
)

type v1Handler struct {
	service *session.V1Service
}

func (h *v1Handler) Create(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	defer r.Body.Close()

	req := v1.Session{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	s, err := h.service.Create(ctx, req.ProjectId, req.SpaceId)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, s)
}

func (h *v1Handler) AddMessage(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	defer r.Body.Close()

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["session_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Session ID")
		return
	}

	req := v1.Message{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.service.AddMessage(ctx, id, req.Role, req.Content); err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *v1Handler) FinishSession(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["session_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Session ID")
		return
	}

	if err := h.service.FinishSession(ctx, id); err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func NewV1Handler(s *session.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
