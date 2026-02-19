package v1space

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	"github.com/w-h-a/gomento/internal/service"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
)

type v1Handler struct {
	service *v1space.V1Service
}

func (h *v1Handler) Create(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	s, err := h.service.Create(r.Context(), req.Name)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, s)
}

func (h *v1Handler) List(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.List(r.Context())
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, out)
}

func (h *v1Handler) Get(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["space_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	space, err := h.service.Get(r.Context(), id)
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
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["space_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	q := r.URL.Query().Get("q")
	if len(q) == 0 {
		httphandler.WrtErr(w, http.StatusBadRequest, "Query parameter is required")
		return
	}

	var limit int
	if l := r.URL.Query().Get("limit"); len(l) > 0 {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	skills, err := h.service.SearchSkills(r.Context(), id, q, limit)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, skills)
}

func (h *v1Handler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["space_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	q := r.URL.Query().Get("q")
	if len(q) == 0 {
		httphandler.WrtErr(w, http.StatusBadRequest, "Query parameter is required")
		return
	}

	var limit int
	if l := r.URL.Query().Get("limit"); len(l) > 0 {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	msgs, err := h.service.SearchMessages(r.Context(), id, q, limit)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, msgs)
}

func (h *v1Handler) SearchFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["space_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	q := r.URL.Query().Get("q")
	if len(q) == 0 {
		httphandler.WrtErr(w, http.StatusBadRequest, "Query parameter is required")
		return
	}

	var limit int
	if l := r.URL.Query().Get("limit"); len(l) > 0 {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	files, err := h.service.SearchFiles(r.Context(), id, q, limit)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, files)
}

func (h *v1Handler) SearchChunks(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["space_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	q := r.URL.Query().Get("q")
	if len(q) == 0 {
		httphandler.WrtErr(w, http.StatusBadRequest, "Query parameter is required")
		return
	}

	var limit int
	if l := r.URL.Query().Get("limit"); len(l) > 0 {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	chunks, err := h.service.SearchChunks(r.Context(), id, q, limit)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, chunks)
}

func NewV1Handler(s *v1space.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
