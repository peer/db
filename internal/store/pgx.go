package pgx

import (
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
)

func ErrorDetails(e *pgconn.PgError) map[string]interface{} {
	details := map[string]interface{}{}
	if e.Severity != "" {
		details["severity"] = e.Severity
	}
	if e.Code != "" {
		details["code"] = e.Code
	}
	if e.Message != "" {
		// We use zerolog.MessageFieldName here so that when notice (which is really just PgError)
		// is logged, its message becomes log line's message.
		details[zerolog.MessageFieldName] = e.Message
	}
	if e.Detail != "" {
		details["details"] = e.Detail
	}
	if e.Hint != "" {
		details["hint"] = e.Hint
	}
	if e.Position != 0 {
		details["position"] = e.Position
	}
	if e.InternalPosition != 0 {
		details["internalPosition"] = e.InternalPosition
	}
	if e.InternalQuery != "" {
		details["internalQuery"] = e.InternalQuery
	}
	if e.Where != "" {
		details["where"] = e.Where
	}
	if e.SchemaName != "" {
		details["schemaName"] = e.SchemaName
	}
	if e.TableName != "" {
		details["tableName"] = e.TableName
	}
	if e.ColumnName != "" {
		details["columnName"] = e.ColumnName
	}
	if e.DataTypeName != "" {
		details["dataTypeName"] = e.DataTypeName
	}
	if e.ConstraintName != "" {
		details["constraintName"] = e.ConstraintName
	}
	if e.File != "" {
		details["file"] = e.File
	}
	if e.Line != 0 {
		details["line"] = e.Line
	}
	if e.Routine != "" {
		details["routine"] = e.Routine
	}
	return details
}

func WithPgxError(err error) errors.E {
	errE := errors.WithStack(err)
	var e *pgconn.PgError
	if errors.As(err, &e) {
		details := errors.Details(errE)
		for key, value := range ErrorDetails(e) {
			details[key] = value
		}
	}
	return errE
}
