package v1session

import (
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
	"github.com/w-h-a/gomento/internal/util"
)

type v1Handler struct {
	service *v1session.V1Service
}

func (h *v1Handler) Create(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	defer r.Body.Close()

	var req struct {
		ProjectId string  `json:"project_id"`
		SpaceId   *string `json:"space_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	pid, err := uuid.Parse(req.ProjectId)
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid UUIDs")
		return
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

	s, err := h.service.Create(ctx, pid, sid)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, s)
}

func (h *v1Handler) ConnectToSpace(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	sessionId, err := uuid.Parse(vars["session_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Session ID")
		return
	}

	var req struct {
		SpaceId string `json:"space_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	sid, err := uuid.Parse(req.SpaceId)
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	if err := h.service.ConnectToSpace(ctx, sessionId, sid); err != nil {
		if errors.Is(err, v1session.ErrSessionNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "Session not found")
			return
		}
		if errors.Is(err, v1session.ErrSpaceNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "Space not found")
			return
		}
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, map[string]string{"status": "connected"})
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

	partsStr := r.FormValue("parts")
	var parts []v1session.PartInput
	if err := json.Unmarshal([]byte(partsStr), &parts); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid parts JSON")
		return
	}

	fileMap := make(map[string]*multipart.FileHeader)
	if r.MultipartForm != nil {
		for name, files := range r.MultipartForm.File {
			if len(files) > 0 {
				fileMap[name] = files[0]
			}
		}
	}

	input := v1session.SendMessageInput{
		SessionId: id,
		Role:      r.FormValue("role"),
		Parts:     parts,
		Files:     fileMap,
	}

	msg, err := h.service.AddMessage(ctx, input)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, msg)
}

func (h *v1Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["session_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Session ID")
		return
	}

	var limit int
	if l := r.URL.Query().Get("limit"); len(l) > 0 {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	cursor := r.URL.Query().Get("cursor")
	withPublicUrl := r.URL.Query().Get("with_asset_public_url") == "true"

	out, err := h.service.GetMessages(ctx, v1session.GetMessagesInput{
		SessionId:          id,
		Limit:              limit,
		Cursor:             cursor,
		WithAssetPublicUrl: withPublicUrl,
	})
	if err != nil {
		if errors.Is(err, util.ErrInvalidCursor) {
			httphandler.WrtErr(w, http.StatusBadRequest, "Invalid cursor format")
			return
		}
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, out)
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

func NewV1Handler(s *v1session.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
