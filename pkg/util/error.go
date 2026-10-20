package util

import (
	"errors"
	"net/http"
)

// AppError is a typed application error carrying an HTTP status code, a
// human readable message and the wrapped cause. Services return these so the
// handler layer never needs to inspect raw errors.
type AppError struct {
	Message string
	Status  int
	Err     error
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap exposes the wrapped cause for errors.Is / errors.As.
func (e *AppError) Unwrap() error {
	return e.Err
}

// newAppError builds an AppError with a status code, uppercasing nothing so
// callers remain in full control of the message.
func newAppError(message string, status int, err error) *AppError {
	return &AppError{
		Message: message,
		Status:  status,
		Err:     err,
	}
}

// NewBadRequestError returns a 400 typed error.
func NewBadRequestError(message string, err error) *AppError {
	return newAppError(message, http.StatusBadRequest, err)
}

// NewUnauthorizedError returns a 401 typed error.
func NewUnauthorizedError(message string, err error) *AppError {
	return newAppError(message, http.StatusUnauthorized, err)
}

// NewForbiddenError returns a 403 typed error.
func NewForbiddenError(message string, err error) *AppError {
	return newAppError(message, http.StatusForbidden, err)
}

// NewNotFoundError returns a 404 typed error.
func NewNotFoundError(message string, err error) *AppError {
	return newAppError(message, http.StatusNotFound, err)
}

// NewInternalServerError returns a 500 typed error.
func NewInternalServerError(message string, err error) *AppError {
	return newAppError(message, http.StatusInternalServerError, err)
}

// GetStatusCode maps any error to its HTTP status, defaulting to 500.
func GetStatusCode(err error) int {
	if appErr, ok := asAppError(err); ok {
		return appErr.Status
	}
	return http.StatusInternalServerError
}

// GetErrorMessage returns the human readable message of a typed error. Raw
// errors are hidden behind a generic message to avoid leaking internals.
func GetErrorMessage(err error) string {
	if appErr, ok := asAppError(err); ok {
		return appErr.Message
	}
	return "An unexpected error occurred"
}

// asAppError converts any error into an *AppError using errors.As.
func asAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if err == nil {
		return nil, false
	}
	if !errors.As(err, &appErr) {
		return nil, false
	}
	return appErr, true
}
