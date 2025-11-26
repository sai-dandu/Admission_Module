package errors

import (
	// Go internal packages
	"bytes"
	"encoding/json"
	"errors"
)

// Error defines a standard application error.
type Error struct {
	Kind    Kind   `json:"kind"`
	Message string `json:"message"`
	// Wrapped underlying error.
	WrappedErr error `json:"wrapped_err,omitempty"`
}

// Error returns the string representation of the error message.
func (e *Error) Error() string {
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(e)
	return buf.String()
}

// Unwrap returns the wrapped error.
func (e *Error) Unwrap() error {
	return e.WrappedErr
}

// NewError returns standard go error with given string
func NewError(e string) error {
	return errors.New(e)
}

// Kind defines the kind or class of an error.
type Kind uint8

// Transport agnostic error "kinds"
const (
	Other        Kind = iota // Unclassified error
	Internal                 // Internal error
	Conflict                 // Conflict when an entity already exists
	Invalid                  // Invalid input, validation error etc
	NotFound                 // Entity does not exist
	Unauthorized             // Unauthorized access
	Forbidden                // Forbidden access
)

func (k Kind) String() string {
	switch k {
	case Other:
		return "unclassified error"
	case Internal:
		return "internal error"
	case Invalid:
		return "invalid input"
	case NotFound:
		return "entity not found"
	default:
		return "unknown error kind"
	}
}

func (k Kind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

func E(args ...interface{}) error {
	e := &Error{}
	for _, arg := range args {
		switch arg := arg.(type) {
		case Kind:
			e.Kind = arg
		case error:
			e.WrappedErr = arg
		case string:
			e.Message = arg
		}
	}
	return e
}

// NewInternalServerError creates a new internal server error
func NewInternalServerError(msg string) error {
	return E(Internal, msg)
}

// NewNotFoundError creates a new not found error
func NewNotFoundError(msg string) error {
	return E(NotFound, msg)
}

// NewInvalidParamsError creates a new invalid parameters error
func NewInvalidParamsError(msg string) error {
	return E(Invalid, msg)
}

// NewUnauthorizedError creates a new unauthorized error
func NewUnauthorizedError(msg string) error {
	return E(Unauthorized, msg)
}

// NewForbiddenError creates a new forbidden error
func NewForbiddenError(msg string) error {
	return E(Forbidden, msg)
}

// NewConflictError creates a new conflict error
func NewConflictError(msg string) error {
	return E(Conflict, msg)
}

var (
	As = errors.As
	Is = errors.Is
)
