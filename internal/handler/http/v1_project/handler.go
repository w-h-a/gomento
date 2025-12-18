package v1project

import (
	"encoding/json"
	"net/http"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	v1project "github.com/w-h-a/gomento/internal/service/v1_project"
	"github.com/w-h-a/gomento/internal/util"
)

type v1Handler struct {
	service *v1project.V1Service
}

func (h *v1Handler) Create(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	defer r.Body.Close()

	req := v1.Project{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	p, err := h.service.Create(ctx, req.Name)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, p)
}

func NewV1Handler(s *v1project.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
