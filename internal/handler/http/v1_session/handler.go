package v1session

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	"github.com/w-h-a/gomento/internal/service"
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
		SpaceId *string `json:"space_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
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

	s, err := h.service.Create(ctx, sid)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, s)
}

func (h *v1Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	var sid *uuid.UUID
	if s := r.URL.Query().Get("space_id"); len(s) > 0 {
		id, err := uuid.Parse(s)
		if err != nil {
			httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
			return
		}
		sid = &id
	}

	out, err := h.service.ListSessions(ctx, v1session.ListSessionsInput{
		SpaceId: sid,
	})
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, out)
}

func (h *v1Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["session_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Session ID")
		return
	}

	sess, err := h.service.GetSession(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "Session not found")
			return
		}
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, sess)
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
		if errors.Is(err, service.ErrSessionNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "Session not found")
			return
		}
		if errors.Is(err, service.ErrSpaceNotFound) {
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

	role := r.FormValue("role")

	partsStr := r.FormValue("parts")

	var parts []v1session.PartInput
	if err := json.Unmarshal([]byte(partsStr), &parts); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid parts JSON")
		return
	}

	inputFiles := make(map[string]v1session.InputFile)

	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()

	if r.MultipartForm != nil {
		for name, files := range r.MultipartForm.File {
			if len(files) > 0 {
				fh := files[0]

				f, err := fh.Open()
				if err != nil {
					httphandler.WrtErr(w, http.StatusInternalServerError, "failed to open file")
					return
				}

				closers = append(closers, f)

				inputFiles[name] = v1session.InputFile{
					Filename:    fh.Filename,
					ContentType: fh.Header.Get("Content-Type"),
					Size:        fh.Size,
					Reader:      f,
				}
			}
		}
	}

	input := v1session.SendMessageInput{
		SessionId: id,
		Role:      role,
		Parts:     parts,
		Files:     inputFiles,
	}

	msg, err := h.service.AddMessage(ctx, input)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, msg)
}

func (h *v1Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
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

	out, err := h.service.ListMessages(ctx, v1session.ListMessagesInput{
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

func (h *v1Handler) CheckpointSession(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["session_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Session ID")
		return
	}

	if err := h.service.CheckpointSession(ctx, id); err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusAccepted, map[string]string{"status": "checkpoint_initiated"})
}

func (h *v1Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["session_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Session ID")
		return
	}

	var status *string
	if s := r.URL.Query().Get("status"); s != "" {
		status = &s
	}

	out, err := h.service.ListTasks(ctx, v1session.ListTasksInput{
		SessionId: id,
		Status:    status,
	})
	if err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "Session not found")
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

	httphandler.WrtJSON(w, http.StatusAccepted, map[string]string{"status": "finish_initiated"})
}

func NewV1Handler(s *v1session.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
