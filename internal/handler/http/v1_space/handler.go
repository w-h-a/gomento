package v1space

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	"github.com/w-h-a/gomento/internal/service"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
	"github.com/w-h-a/gomento/internal/util"
)

type v1Handler struct {
	service *v1space.V1Service
}

func (h *v1Handler) Create(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	defer r.Body.Close()

	var req struct {
		ProjectId string `json:"project_id"`
		Name      string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	pid, err := uuid.Parse(req.ProjectId)
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Project ID")
		return
	}

	s, err := h.service.Create(ctx, pid, req.Name)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, s)
}

func (h *v1Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	spaceId, err := uuid.Parse(vars["space_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "invalid space id")
		return
	}

	var status *string
	if s := r.URL.Query().Get("status"); s != "" {
		status = &s
	}

	out, err := h.service.ListTasks(ctx, v1space.ListTasksInput{
		SpaceId: spaceId,
		Status:  status,
	})
	if err != nil {
		if errors.Is(err, service.ErrSpaceNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "Space not found")
			return
		}
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, out)
}

func NewV1Handler(s *v1space.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
