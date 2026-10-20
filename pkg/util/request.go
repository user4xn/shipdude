package util

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RespondJSON writes the uniform response envelope for a JSON handler.
func RespondJSON(w http.ResponseWriter, status int, message string, data any) {
	WriteJSON(w, APIResponse(message, status, data))
}

// RespondError maps any error to its status code and message and writes the
// uniform envelope.
func RespondError(w http.ResponseWriter, err error) {
	status := GetStatusCode(err)
	WriteJSON(w, APIResponse(GetErrorMessage(err), status, nil))
}

// URLParam reads a path parameter from the chi route context.
func URLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

// DecodeJSON decodes a JSON request body into target.
func DecodeJSON(r *http.Request, target any) error {
	return json.NewDecoder(r.Body).Decode(target)
}
