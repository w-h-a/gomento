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
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	s, err := h.service.Create(ctx, req.Name)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, s)
}

func (h *v1Handler) ListSpaces(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	out, err := h.service.ListSpaces(ctx)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, out)
}

func (h *v1Handler) GetSpace(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["space_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	space, err := h.service.GetSpace(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrSpaceNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "Space not found")
			return
		}
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, space)
}

func (h *v1Handler) SearchSkills(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["space_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	q := r.URL.Query().Get("q")

	skills, err := h.service.SearchSkills(ctx, id, q)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, skills)
}

func (h *v1Handler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["space_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	q := r.URL.Query().Get("q")

	msgs, err := h.service.SearchMessages(ctx, id, q)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, msgs)
}

func NewV1Handler(s *v1space.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
