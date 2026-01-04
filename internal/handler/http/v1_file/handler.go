package v1file

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	httphandler "github.com/w-h-a/gomento/internal/handler/http"
	"github.com/w-h-a/gomento/internal/service"
	v1file "github.com/w-h-a/gomento/internal/service/v1_file"
	"github.com/w-h-a/gomento/internal/util"
)

type v1Handler struct {
	service *v1file.V1Service
}

func (h *v1Handler) UploadFile(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	defer r.Body.Close()

	var sid *uuid.UUID
	if s := r.URL.Query().Get("space_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
			return
		}
		sid = &id
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

	f, err := h.service.UploadFile(ctx, v1file.CreateFileInput{
		SpaceId:  sid,
		Path:     logicPath,
		Filename: header.Filename,
		MimeType: header.Header.Get("Content-Type"),
		Size:     header.Size,
		Reader:   file,
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

	pathPrefix := r.URL.Query().Get("path_prefix")

	var sid *uuid.UUID
	if s := r.URL.Query().Get("space_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
			return
		}
		sid = &id
	}

	out, err := h.service.ListFiles(ctx, v1file.ListFilesInput{
		SpaceId:    sid,
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
	id, err := uuid.Parse(vars["file_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid File ID")
		return
	}

	withUrl := r.URL.Query().Get("with_url") == "true"

	file, url, err := h.service.GetFile(ctx, id, withUrl)
	if err != nil {
		if errors.Is(err, service.ErrFileNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "File not found")
			return
		}
		httphandler.WrtErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	rsp := map[string]any{
		"file": file,
	}

	if url != "" {
		rsp["public_url"] = url
	}

	httphandler.WrtJSON(w, http.StatusOK, rsp)
}

func (h *v1Handler) ConnectToSpace(w http.ResponseWriter, r *http.Request) {
	traceId := httphandler.GetTraceId(r)
	ctx := util.WithTraceId(r.Context(), traceId)

	vars := mux.Vars(r)
	fileId, err := uuid.Parse(vars["file_id"])
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid File ID")
		return
	}

	var req struct {
		SpaceId string `json:"space_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	spaceId, err := uuid.Parse(req.SpaceId)
	if err != nil {
		httphandler.WrtErr(w, http.StatusBadRequest, "Invalid Space ID")
		return
	}

	if err := h.service.ConnectToSpace(ctx, fileId, spaceId); err != nil {
		if errors.Is(err, service.ErrFileNotFound) {
			httphandler.WrtErr(w, http.StatusNotFound, "File not found")
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

func NewV1Handler(s *v1file.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
