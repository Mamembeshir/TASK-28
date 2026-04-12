package model

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrStaleVersion = errors.New("stale version: resource has been modified by another user")
	ErrValidation   = errors.New("validation error")
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationErrors struct {
	Errors []ValidationError
}

func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}
	return e.Errors[0].Message
}

func (e *ValidationErrors) Add(field, message string) {
	e.Errors = append(e.Errors, ValidationError{Field: field, Message: message})
}

func (e *ValidationErrors) HasErrors() bool {
	return len(e.Errors) > 0
}

func NewValidationErrors() *ValidationErrors {
	return &ValidationErrors{}
}
