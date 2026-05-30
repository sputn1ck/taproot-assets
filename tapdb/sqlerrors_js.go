//go:build js && wasm

package tapdb

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
)

var (
	// ErrRetriesExceeded is returned when a transaction is retried more
	// than the max allowed valued without a success.
	ErrRetriesExceeded = errors.New("db tx retries exceeded")
)

// MapSQLError attempts to interpret a given error as a database agnostic SQL
// error.
func MapSQLError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "UNIQUE constraint failed"),
		strings.Contains(errMsg, "constraint failed"):

		return &ErrSqlUniqueConstraintViolation{DbError: err}

	case strings.Contains(errMsg, "SQLITE_BUSY"):
		return &ErrSerializationError{DbError: err}

	case strings.Contains(errMsg, "database is locked"),
		strings.Contains(errMsg, "SQLITE_LOCKED"),
		strings.Contains(errMsg, "SQLITE_BUSY_SNAPSHOT"):

		return &ErrDeadlockError{DbError: err}

	case strings.Contains(errMsg, "no such table"):
		return &ErrSchemaError{DbError: err}
	}

	var pqErr *pgconn.PgError
	if errors.As(err, &pqErr) {
		return parsePostgresError(pqErr)
	}

	return sanitizeConnectionError(err)
}

// parsePostgresError attempts to parse a postgres error as a database agnostic
// SQL error.
func parsePostgresError(pqErr *pgconn.PgError) error {
	switch pqErr.Code {
	case pgerrcode.UniqueViolation:
		return &ErrSqlUniqueConstraintViolation{
			DbError: pqErr,
		}

	case pgerrcode.SerializationFailure:
		return &ErrSerializationError{
			DbError: pqErr,
		}

	case pgerrcode.DeadlockDetected:
		return &ErrDeadlockError{
			DbError: pqErr,
		}

	case pgerrcode.UndefinedColumn, pgerrcode.UndefinedTable:
		return &ErrSchemaError{
			DbError: pqErr,
		}

	default:
		return fmt.Errorf("unknown postgres error: %w",
			sanitizeConnectionError(pqErr))
	}
}

// ErrSqlUniqueConstraintViolation is an error type which represents a database
// agnostic SQL unique constraint violation.
type ErrSqlUniqueConstraintViolation struct {
	DbError error
}

func (e ErrSqlUniqueConstraintViolation) Error() string {
	return fmt.Sprintf("sql unique constraint violation: %v", e.DbError)
}

// ErrSerializationError is an error type which represents a database agnostic
// error that a transaction couldn't be serialized with other concurrent db
// transactions.
type ErrSerializationError struct {
	DbError error
}

// Unwrap returns the wrapped error.
func (e ErrSerializationError) Unwrap() error {
	return e.DbError
}

// Error returns the error message.
func (e ErrSerializationError) Error() string {
	return e.DbError.Error()
}

// ErrDeadlockError is an error type which represents a database agnostic
// error where transactions have led to cyclic dependencies in lock acquisition.
type ErrDeadlockError struct {
	DbError error
}

// Unwrap returns the wrapped error.
func (e ErrDeadlockError) Unwrap() error {
	return e.DbError
}

// Error returns the error message.
func (e ErrDeadlockError) Error() string {
	return e.DbError.Error()
}

// IsSerializationError returns true if the given error is a serialization
// error.
func IsSerializationError(err error) bool {
	var serializationError *ErrSerializationError
	return errors.As(err, &serializationError)
}

// IsDeadlockError returns true if the given error is a deadlock error.
func IsDeadlockError(err error) bool {
	var deadlockError *ErrDeadlockError
	return errors.As(err, &deadlockError)
}

// IsSerializationOrDeadlockError returns true if the given error is either a
// deadlock error or a serialization error.
func IsSerializationOrDeadlockError(err error) bool {
	return IsDeadlockError(err) || IsSerializationError(err)
}

// ErrSchemaError is an error type which represents a database agnostic error
// that the schema of the database is incorrect for the given query.
type ErrSchemaError struct {
	DbError error
}

// Unwrap returns the wrapped error.
func (e ErrSchemaError) Unwrap() error {
	return e.DbError
}

// Error returns the error message.
func (e ErrSchemaError) Error() string {
	return e.DbError.Error()
}

// IsSchemaError returns true if the given error is a schema error.
func IsSchemaError(err error) bool {
	var schemaError *ErrSchemaError
	return errors.As(err, &schemaError)
}

// ErrDatabaseConnectionError is an error type which represents a database
// connection error with sensitive information sanitized.
type ErrDatabaseConnectionError struct {
	DbError error
}

// Unwrap returns the wrapped error.
func (e ErrDatabaseConnectionError) Unwrap() error {
	return e.DbError
}

// Error returns a generic error message without revealing connection details.
func (e ErrDatabaseConnectionError) Error() string {
	return "database connection failed"
}

func isConnectionError(errStr string) bool {
	patterns := []string{
		"failed to connect to",
		"dial tcp",
		"user=",
		"password=",
		"host=",
		"dbname=",
		"sslmode=",
		"connection refused",
		"no route to host",
		"password authentication failed",
	}

	for _, pattern := range patterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

func sanitizeConnectionError(err error) error {
	if err == nil {
		return nil
	}

	if isConnectionError(err.Error()) {
		log.Errorf("Database connection error (sanitized): %v", err)
		return &ErrDatabaseConnectionError{
			DbError: err,
		}
	}

	return err
}
