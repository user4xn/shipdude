package util

import (
	"encoding/json"
	"net/http"
)

// Response is the uniform API envelope returned by every JSON endpoint.
type Response struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// APIResponse builds the envelope. For error statuses (>= 400) the message is
// also mirrored into the `error` field so legacy clients that read
// `payload.error` keep working unchanged.
func APIResponse(message string, status int, data any) *Response {
	res := &Response{
		Message: message,
		Status:  status,
		Data:    data,
	}

	if status >= http.StatusBadRequest {
		res.Error = message
	}

	return res
}

// WriteJSON writes the envelope to the response writer. It exists so the
// package can serve the few handlers that are not Gin based.
func WriteJSON(w http.ResponseWriter, res *Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.Status)
	_ = json.NewEncoder(w).Encode(res)
}
