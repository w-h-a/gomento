package v1artifact

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	"github.com/w-h-a/gomento/internal/service"
	v1artifact "github.com/w-h-a/gomento/internal/service/v1_artifact"
	"github.com/w-h-a/gomento/internal/util"
)

type v1Handler struct {
	service *v1artifact.V1Service
}

func (h *v1Handler) Create(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	defer r.Body.Close()

	var req struct {
		ProjectId string `json:"project_id"`
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

	s, err := h.service.Create(ctx, pid)
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusCreated, s)
}

func (h *v1Handler) UploadFile(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	defer r.Body.Close()

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["artifact_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Artifact ID")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Missing 'file' part")
		return
	}
	defer file.Close()

	logicPath := r.FormValue("path")
	if logicPath == "" {
		logicPath = "/"
	}

	f, err := h.service.UploadFile(ctx, v1artifact.CreateFileInput{
		ArtifactId: id,
		Path:       logicPath,
		Filename:   header.Filename,
		MimeType:   header.Header.Get("Content-Type"),
		Size:       header.Size,
		Reader:     file,
	})
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, f)
}

func (h *v1Handler) ListFiles(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["artifact_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Artifact ID")
		return
	}

	pathPrefix := r.URL.Query().Get("path")

	out, err := h.service.ListFiles(ctx, v1artifact.ListFilesInput{
		ArtifactId: id,
		PathPrefix: pathPrefix,
	})
	if err != nil {
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	httphandler.WrtJSON(w, http.StatusOK, out)
}

func (h *v1Handler) GetFile(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["artifact_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Artifact ID")
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		httphandler.WrtErr(w, http.StatusBadRequest, "Missing 'path' query parameter")
		return
	}

	withUrl := r.URL.Query().Get("with_url") == "true"

	file, url, err := h.service.GetFile(ctx, id, path, withUrl)
	if err != nil {
		if errors.Is(err, service.ErrFileNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "File not found")
			return
		}
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]any{
		"file": file,
	}

	if url != "" {
		resp["public_url"] = url
	}

	httphandler.WrtJSON(w, http.StatusOK, resp)
}

func NewV1Handler(s *v1artifact.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
