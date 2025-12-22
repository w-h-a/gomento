package v1session

import (
	"encoding/json"
	"mime/multipart"
	"net/http"

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
		ProjectId string `json:"project_id"`
		SpaceId   string `json:"space_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	pid, err := uuid.Parse(req.ProjectId)
	sid, err := uuid.Parse(req.SpaceId)
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid UUIDs")
		return
	}

	s, err := h.service.Create(ctx, pid, sid)
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
