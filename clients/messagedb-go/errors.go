package messagedb

import "fmt"

// Error represents a MessageDB error
type Error struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Error implements the error interface
func (e *Error) Error() string {
	if len(e.Details) > 0 {
		return fmt.Sprintf("%s: %s (details: %v)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Is allows error comparison
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// Common error codes
var (
	ErrAuthRequired      = &Error{Code: "AUTH_REQUIRED", Message: "authentication required"}
	ErrAuthInvalid       = &Error{Code: "AUTH_INVALID", Message: "invalid authentication"}
	ErrNamespaceExists   = &Error{Code: "NAMESPACE_EXISTS", Message: "namespace already exists"}
	ErrNamespaceNotFound = &Error{Code: "NAMESPACE_NOT_FOUND", Message: "namespace not found"}
	ErrVersionConflict   = &Error{Code: "STREAM_VERSION_CONFLICT", Message: "stream version conflict"}
	ErrInvalidRequest    = &Error{Code: "INVALID_REQUEST", Message: "invalid request"}
)
