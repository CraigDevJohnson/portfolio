package app

import (
	"encoding/json"
	"net/http"
)

func healthHandler(revision string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"revision": revision,
			"status":   "ok",
		})
	}
}
