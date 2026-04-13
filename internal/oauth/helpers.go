package oauth

import (
	"encoding/json"
	"net/http"
)

func writeOAuthError(w http.ResponseWriter, status int, code string) {
	if code == "" {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, map[string]any{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
