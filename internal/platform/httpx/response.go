package httpx

import (
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Error     apiError `json:"error"`
	RequestID string   `json:"request_id"`
}
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	// Encode before committing headers so encoding failure cannot produce partial JSON.
	body, err := json.Marshal(v)
	if err != nil {
		panic("response encoding failed")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// A disconnected client cannot receive an alternative response.
	_, _ = w.Write(append(body, '\n'))
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: apiError{Code: code, Message: message}, RequestID: RequestID(r.Context())})
}
