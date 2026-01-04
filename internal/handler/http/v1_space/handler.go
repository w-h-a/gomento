package v1space

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
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

func NewV1Handler(s *v1space.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
