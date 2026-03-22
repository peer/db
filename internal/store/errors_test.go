package store_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

func TestErrorDetailsAllFields(t *testing.T) {
	t.Parallel()

	e := &pgconn.PgError{
		Severity:            "ERROR",
		SeverityUnlocalized: "ERROR",
		Code:                "23505",
		Message:             "duplicate key value violates unique constraint",
		Detail:              "Key (id)=(1) already exists.",
		Hint:                "Try a different value.",
		Position:            42,
		InternalPosition:    10,
		InternalQuery:       "SELECT 1",
		Where:               "SQL function",
		SchemaName:          "public",
		TableName:           "users",
		ColumnName:          "id",
		DataTypeName:        "integer",
		ConstraintName:      "users_pkey",
		File:                "nbtinsert.c",
		Line:                563,
		Routine:             "_bt_check_unique",
	}

	details := internalStore.ErrorDetails(e)

	assert.Equal(t, "ERROR", details["severity"])
	assert.Equal(t, "23505", details["code"])
	assert.Equal(t, "duplicate key value violates unique constraint", details["message"])
	assert.Equal(t, "Key (id)=(1) already exists.", details["details"])
	assert.Equal(t, "Try a different value.", details["hint"])
	assert.Equal(t, int32(42), details["position"])
	assert.Equal(t, int32(10), details["internalPosition"])
	assert.Equal(t, "SELECT 1", details["internalQuery"])
	assert.Equal(t, "SQL function", details["where"])
	assert.Equal(t, "public", details["schemaName"])
	assert.Equal(t, "users", details["tableName"])
	assert.Equal(t, "id", details["columnName"])
	assert.Equal(t, "integer", details["dataTypeName"])
	assert.Equal(t, "users_pkey", details["constraintName"])
	assert.Equal(t, "nbtinsert.c", details["file"])
	assert.Equal(t, int32(563), details["line"])
	assert.Equal(t, "_bt_check_unique", details["routine"])
}

func TestErrorDetailsEmptyFields(t *testing.T) {
	t.Parallel()

	e := &pgconn.PgError{}

	details := internalStore.ErrorDetails(e)

	assert.Empty(t, details)
}

func TestErrorDetailsPartialFields(t *testing.T) {
	t.Parallel()

	e := &pgconn.PgError{ //nolint:exhaustruct
		Severity: "WARNING",
		Code:     "42P01",
		Message:  "relation does not exist",
	}

	details := internalStore.ErrorDetails(e)

	assert.Len(t, details, 3)
	assert.Equal(t, "WARNING", details["severity"])
	assert.Equal(t, "42P01", details["code"])
	assert.Equal(t, "relation does not exist", details["message"])
}

func TestWithPgxErrorNil(t *testing.T) {
	t.Parallel()

	errE := internalStore.WithPgxError(nil)
	assert.Nil(t, errE)
}

func TestWithPgxErrorPlainError(t *testing.T) {
	t.Parallel()

	err := errors.New("plain error")
	errE := internalStore.WithPgxError(err)
	require.NotNil(t, errE)
	assert.ErrorIs(t, errE, err)
}

func TestWithPgxErrorPgError(t *testing.T) {
	t.Parallel()

	pgErr := &pgconn.PgError{ //nolint:exhaustruct
		Severity: "ERROR",
		Code:     "23505",
		Message:  "unique violation",
	}

	errE := internalStore.WithPgxError(pgErr)
	require.NotNil(t, errE)

	details := errors.Details(errE)
	assert.Equal(t, "ERROR", details["severity"])
	assert.Equal(t, "23505", details["code"])
	assert.Equal(t, "unique violation", details["message"])
}
