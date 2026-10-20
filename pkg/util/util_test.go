package util

import (
	"errors"
	"net/http"
	"testing"
)

func TestGetStatusCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"bad request", NewBadRequestError("bad", nil), http.StatusBadRequest},
		{"unauthorized", NewUnauthorizedError("nope", nil), http.StatusUnauthorized},
		{"forbidden", NewForbiddenError("no", nil), http.StatusForbidden},
		{"not found", NewNotFoundError("missing", nil), http.StatusNotFound},
		{"internal", NewInternalServerError("boom", nil), http.StatusInternalServerError},
		{"raw", errors.New("raw"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		if got := GetStatusCode(tc.err); got != tc.want {
			t.Errorf("%s: expected %d, got %d", tc.name, tc.want, got)
		}
	}
}

func TestGetErrorMessage(t *testing.T) {
	wrapped := NewNotFoundError("Project not found", errors.New("sql: no rows"))
	if got := GetErrorMessage(wrapped); got != "Project not found" {
		t.Errorf("expected typed message, got %q", got)
	}

	if got := GetErrorMessage(errors.New("secret internal detail")); got != "An unexpected error occurred" {
		t.Errorf("expected raw errors to be hidden, got %q", got)
	}
}

func TestAppError_Unwrap(t *testing.T) {
	root := errors.New("root cause")
	appErr := NewInternalServerError("Failed to save", root)

	if !errors.Is(appErr, root) {
		t.Errorf("expected errors.Is to reach the wrapped cause")
	}
}

func TestAPIResponse_ErrorMirrorsMessage(t *testing.T) {
	ok := APIResponse("Successfully listed projects", http.StatusOK, []string{})
	if ok.Error != "" || ok.Message != "Successfully listed projects" {
		t.Errorf("unexpected success envelope: %+v", ok)
	}

	fail := APIResponse("Project not found", http.StatusNotFound, nil)
	if fail.Error != "Project not found" || fail.Status != http.StatusNotFound {
		t.Errorf("expected error field mirrored for legacy clients, got %+v", fail)
	}
}

func TestGetEnv(t *testing.T) {
	if got := GetEnv("SHIPDUDE_UNSET_KEY_FOR_TEST", "fallback"); got != "fallback" {
		t.Errorf("expected fallback, got %q", got)
	}
	if got := GetEnvInt("SHIPDUDE_UNSET_INT_FOR_TEST", 22); got != 22 {
		t.Errorf("expected 22, got %d", got)
	}
}
