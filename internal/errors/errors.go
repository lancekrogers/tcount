// Package errors provides structured error types for tcount.
package errors

import (
	"fmt"
)

// Error codes for categorization.
const (
	ErrCodeNotFound   = "NOT_FOUND"
	ErrCodeValidation = "VALIDATION"
	ErrCodeIO         = "IO"
	ErrCodeParse      = "PARSE"
	ErrCodeInternal   = "INTERNAL"
)

// Error is a structured error type with code, context, and chain support.
type Error struct {
	Code    string
	Message string
	Op      string
	Err     error
	Fields  map[string]any
	Hint    string
}

// Error returns the error string with context.
func (e *Error) Error() string {
	var msg string
	if e.Op != "" && e.Err != nil {
		msg = fmt.Sprintf("%s: %s: %v", e.Op, e.Message, e.Err)
	} else if e.Op != "" {
		msg = fmt.Sprintf("%s: %s", e.Op, e.Message)
	} else if e.Err != nil {
		msg = fmt.Sprintf("%s: %v", e.Message, e.Err)
	} else {
		msg = e.Message
	}

	if e.Hint != "" {
		msg = fmt.Sprintf("%s\nHint: %s", msg, e.Hint)
	}
	return msg
}

// Unwrap returns the wrapped error.
func (e *Error) Unwrap() error {
	return e.Err
}

// New creates a new error with a message.
func New(message string) *Error {
	return &Error{
		Code:    ErrCodeInternal,
		Message: message,
		Fields:  make(map[string]any),
	}
}

// Wrap wraps an existing error with a message.
func Wrap(err error, message string) *Error {
	return &Error{
		Code:    ErrCodeInternal,
		Message: message,
		Err:     err,
		Fields:  make(map[string]any),
	}
}

// WithCode sets the error code.
func (e *Error) WithCode(code string) *Error {
	e.Code = code
	return e
}

// WithField adds a context field.
func (e *Error) WithField(key string, value any) *Error {
	if e.Fields == nil {
		e.Fields = make(map[string]any)
	}
	e.Fields[key] = value
	return e
}

// WithHint adds an actionable suggestion to the error.
func (e *Error) WithHint(hint string) *Error {
	e.Hint = hint
	return e
}

// NotFound creates a NOT_FOUND error.
func NotFound(resource string) *Error {
	return &Error{
		Code:    ErrCodeNotFound,
		Message: fmt.Sprintf("%s not found", resource),
		Fields:  map[string]any{"resource": resource},
	}
}

// Validation creates a VALIDATION error.
func Validation(message string) *Error {
	return &Error{
		Code:    ErrCodeValidation,
		Message: message,
		Fields:  make(map[string]any),
	}
}

// IO creates an IO error.
func IO(op string, err error) *Error {
	return &Error{
		Code:    ErrCodeIO,
		Message: "I/O operation failed",
		Op:      op,
		Err:     err,
		Fields:  make(map[string]any),
	}
}

// Parse creates a PARSE error.
func Parse(message string, err error) *Error {
	return &Error{
		Code:    ErrCodeParse,
		Message: message,
		Err:     err,
		Fields:  make(map[string]any),
	}
}

// Code extracts the error code from any error.
func Code(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return ErrCodeInternal
}
