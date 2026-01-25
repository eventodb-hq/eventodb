package store

import (
	"errors"
	"fmt"
)

var (
	// ErrVersionConflict occurs when optimistic locking fails
	ErrVersionConflict = errors.New("version conflict: expected version does not match stream version")

	// ErrNamespaceNotFound occurs when namespace doesn't exist
	ErrNamespaceNotFound = errors.New("namespace not found")

	// ErrPositionExists occurs when trying to import a message with a global position that already exists
	ErrPositionExists = errors.New("global position already exists")

	// ErrNamespaceExists occurs when trying to create a namespace that already exists
	ErrNamespaceExists = errors.New("namespace already exists")

	// ErrStreamNotFound occurs when stream doesn't exist
	ErrStreamNotFound = errors.New("stream not found")

	// ErrInvalidStreamName occurs when stream name format is invalid
	ErrInvalidStreamName = errors.New("invalid stream name format")

	// ErrMigrationFailed occurs when migration execution fails
	ErrMigrationFailed = errors.New("migration failed")

	// ErrInvalidOptions occurs when invalid options are provided
	ErrInvalidOptions = errors.New("invalid options")

	// ErrClosed occurs when operating on a closed store
	ErrClosed = errors.New("store is closed")
)

// VersionConflictError provides detailed information about version conflicts
type VersionConflictError struct {
	StreamName      string
	ExpectedVersion int64
	ActualVersion   int64
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("version conflict on stream %s: expected %d, actual %d",
		e.StreamName, e.ExpectedVersion, e.ActualVersion)
}

func (e *VersionConflictError) Is(target error) bool {
	return target == ErrVersionConflict
}

// NewVersionConflictError creates a new VersionConflictError
func NewVersionConflictError(streamName string, expected, actual int64) error {
	return &VersionConflictError{
		StreamName:      streamName,
		ExpectedVersion: expected,
		ActualVersion:   actual,
	}
}

// IsVersionConflict checks if an error is a version conflict error
func IsVersionConflict(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrVersionConflict)
}
