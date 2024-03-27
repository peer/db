package pgx

import (
	"github.com/jackc/pgx/v5/pgconn"
	"gitlab.com/tozd/go/errors"
)

func WithPgxError(err error) errors.E {
	errE := errors.WithStack(err)
	var e *pgconn.PgError
	if errors.As(err, &e) { //nolint:nestif
		details := errors.Details(errE)
		if e.Severity != "" {
			details["severity"] = e.Severity
		}
		if e.Code != "" {
			details["code"] = e.Code
		}
		if e.Message != "" {
			details["message"] = e.Message
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
	}
	return errE
}
