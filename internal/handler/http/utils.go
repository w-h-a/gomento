package http

import (
	"encoding/json"
	"net/http"
)

func GetTraceId(r *http.Request) string {
	return r.Header.Get("traceparent")
}

func WrtJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func WrtErr(w http.ResponseWriter, status int, msg string) {
	WrtJSON(w, status, map[string]string{"error": msg})
}
