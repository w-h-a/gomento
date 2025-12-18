package space

import (
	"encoding/json"
	"net/http"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	"github.com/w-h-a/gomento/internal/service/space"
	"github.com/w-h-a/gomento/internal/util"
)

type v1Handler struct {
	service *space.V1Service
}

func (h *v1Handler) Create(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	defer r.Body.Close()

	req := v1.Space{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	s, err := h.service.Create(ctx, req.ProjectId, req.Name)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, s)
}

func NewV1Handler(s *space.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
