package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	MaxPageLength    = 5000
	maxPageLengthStr = "5000"
)

// View is not a snapshot of the database but a dynamic named view of a
// set of committed changesets forming a set of values with their history of changes.
//
// Each view can have only one version of a value for a given ID at every committed point in time.
// There might be uncommitted (to the view) divergent versions but when they are committed
// they have all to merge back into one single version (this might mean an additional merge
// changeset has to be introduced which combines multiple parent versions into one version).
//
// All views (except the MainView) depend on ancestor views for all values they do not have
// an explicit version of committed to them. The value is searched for in the ancestry order,
// first the parent view.
type View[Data, Metadata, Patch any] struct {
	name  string
	store *Store[Data, Metadata, Patch]
}

// Name of this named view.
//
// Name is unique across all named views at every point in time. A view can release a
// name and another view can then be created with that name.
func (v View[Data, Metadata, Patch]) Name() string {
	return v.name
}

func (v View[Data, Metadata, Patch]) String() string {
	return v.name
}

// Store returns the Store associated with the view.
//
// It can return nil if Store is not associated with the view.
// You can use WithStore to associate it.
func (v View[Data, Metadata, Patch]) Store() *Store[Data, Metadata, Patch] {
	return v.store
}

// WithStore returns a new View object associated with the given Store.
func (v View[Data, Metadata, Patch]) WithStore(ctx context.Context, store *Store[Data, Metadata, Patch]) (View[Data, Metadata, Patch], errors.E) {
	return store.View(ctx, v.name)
}

// Commit commits a changeset to the view.
func (v View[Data, Metadata, Patch]) Commit(
	ctx context.Context, changeset Changeset[Data, Metadata, Patch], metadata Metadata,
) ([]Changeset[Data, Metadata, Patch], errors.E) {
	return changeset.Commit(ctx, v, metadata)
}

// Insert auto-commits the insert change into the view.
func (v View[Data, Metadata, Patch]) Insert( //nolint:nonamedreturns
	ctx context.Context, id identifier.Identifier, value Data, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.store.Begin(ctx)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Insert(ctx, id, value, metadata)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	_, errE = changeset.Commit(ctx, v, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

// Replace auto-commits the replace change into the view.
func (v View[Data, Metadata, Patch]) Replace( //nolint:nonamedreturns
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.store.Begin(ctx)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Replace(ctx, id, parentChangeset, value, metadata)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	_, errE = changeset.Commit(ctx, v, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

// Update auto-commits the update change into the view.
func (v View[Data, Metadata, Patch]) Update( //nolint:nonamedreturns
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.store.Begin(ctx)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Update(ctx, id, parentChangeset, value, patch, metadata)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	_, errE = changeset.Commit(ctx, v, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

// Merge auto-commits the merge change into the view.
func (v View[Data, Metadata, Patch]) Merge( //nolint:nonamedreturns
	ctx context.Context, id identifier.Identifier, parentChangesets []identifier.Identifier, value Data, patches []Patch, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.store.Begin(ctx)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Merge(ctx, id, parentChangesets, value, patches, metadata)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	_, errE = changeset.Commit(ctx, v, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

// Delete auto-commits the delete change into the view.
func (v View[Data, Metadata, Patch]) Delete( //nolint:nonamedreturns
	ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.store.Begin(ctx)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Delete(ctx, id, parentChangeset, metadata)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	_, errE = changeset.Commit(ctx, v, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

// GetLatest returns the latest committed version of the value for the view.
//
// The latest committed version is not the latest based on the time it was made,
// but that it is the latest in the graph of committed changes to the value
// (i.e., no other change for the value has this version of the value as the parent
// version). Each view can have only one latest committed version for each value.
//
// A view might not have an explicitly committed version of a given value, but its
// ancestor views might. In that case the value is searched for in the ancestry order,
// first the parent view. This means that some further (older) view might have a
// newer value version, but GetLatest still returns the value version which is
// explicitly committed to an earlier (younger) view, i.e., the view shadows values
// and value versions from the parent view for those explicitly committed to the view.
func (v View[Data, Metadata, Patch]) GetLatest(ctx context.Context, id identifier.Identifier) (Data, Metadata, Version, errors.E) { //nolint:ireturn
	arguments := []any{
		v.name, id.String(),
	}
	var data Data
	var metadata Metadata
	var version Version
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		var changeset string
		var revision int64
		var dataIsNull bool
		err := tx.QueryRow(ctx, `
			WITH "viewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.* FROM "currentViews" JOIN "views" USING ("view", "revision"), UNNEST("path") WITH ORDINALITY AS p("view", "depth")
					WHERE "currentViews"."name"=$1
			), "valueView" AS (
				SELECT "view"
					FROM "viewPath" JOIN "committedValues" USING ("view")
					WHERE "id"=$2
					-- We search views for the value in path order. This means that some later (ancestor)
					-- view might have a newer value version, but we still use the one which is explicitly
					-- committed to an earlier view.
					ORDER BY "viewPath"."depth" ASC
					-- We want only the first view with the value.
					LIMIT 1
			)
			SELECT "changeset", "revision", "data", "data" IS NULL, "metadata"
				FROM
					-- This gives us changesets for the value's view.
					"valueView" JOIN "committedValues" USING ("view")
					JOIN (
						-- This gives us current revisions. And corresponding data and metadata.
						"currentChanges" JOIN "changes" USING ("changeset", "id", "revision")
					) USING ("changeset", "id")
				WHERE "id"=$2
					-- We want the latest explicitly committed version of the value.
					-- We know there can be at most one row because we have a CONSTRAINT to ensure that.
					AND "depth"=0
		`, arguments...).Scan(&changeset, &revision, &data, &dataIsNull, &metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// TODO: Is there a better way to check without doing another query?
				var exists bool
				err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "currentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
				if err != nil {
					return errors.Join(errE, internal.WithPgxError(err))
				} else if !exists {
					return errors.WrapWith(errE, ErrViewNotFound)
				}
				return errors.WrapWith(errE, ErrValueNotFound)
			}
			return errE
		}
		version.Changeset = identifier.MustFromString(changeset)
		version.Revision = revision
		if dataIsNull {
			// We return an error because this method is asking for the current version of the value
			// but the value does not exist anymore. Other returned values are valid though.
			return errors.WithStack(ErrValueDeleted)
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = v.name
		details["id"] = id.String()
	}
	return data, metadata, version, errE
}

// Get returns the value at a given version for the view.
//
// Get first searches for the view (including ancestor views) which has the value
// and then returns the value only for versions available for that view.
// This means that some further (older) view might have a
// newer value version, but Get will not return it even if asked for if there is an
// older version explicitly committed to an earlier (younger) view, i.e., the view
// shadows values and value versions from the parent view for those explicitly
// committed to the view.
func (v View[Data, Metadata, Patch]) Get(ctx context.Context, id identifier.Identifier, version Version) (Data, Metadata, errors.E) { //nolint:ireturn
	arguments := []any{
		v.name, id.String(), version.Changeset.String(), version.Revision,
	}
	var data Data
	var metadata Metadata
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		var dataIsNull bool
		err := tx.QueryRow(ctx, `
			WITH "viewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.* FROM "currentViews" JOIN "views" USING ("view", "revision"), UNNEST("path") WITH ORDINALITY AS p("view", "depth")
					WHERE "currentViews"."name"=$1
			), "valueView" AS (
				SELECT "view"
					FROM "viewPath" JOIN "committedValues" USING ("view")
					WHERE "id"=$2
					-- We search views for the value in path order. This means that some later (ancestor)
					-- view might have a newer value version, but we still use the one which is explicitly
					-- committed to an earlier view.
					ORDER BY "viewPath"."depth" ASC
					-- We want only the first view with the value.
					LIMIT 1
			)
			SELECT "data", "data" IS NULL, "metadata"
				FROM
					-- This gives us changesets for the value's view.
					"valueView" JOIN "committedValues" USING ("view")
					-- This gives us corresponding data and metadata.
					JOIN "changes" USING ("changeset", "id")
				WHERE "id"=$2
					AND "changeset"=$3
					AND "revision"=$4
		`, arguments...).Scan(&data, &dataIsNull, &metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// TODO: Is there a better way to check without doing another query?
				var exists bool
				err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "currentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
				if err != nil {
					return errors.Join(errE, internal.WithPgxError(err))
				} else if !exists {
					return errors.WrapWith(errE, ErrViewNotFound)
				}
				return errors.WrapWith(errE, ErrValueNotFound)
			}
			return errE
		}
		if dataIsNull {
			// We return an error because this method is asking for a particular version of the value
			// but the value does not exist anymore at this version. Other returned values are valid though.
			return errors.WithStack(ErrValueDeleted)
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = v.name
		details["id"] = id.String()
		details["changeset"] = version.Changeset.String()
		details["revision"] = version.Revision
	}
	return data, metadata, errE
}

// List returns up to MaxPageLength value IDs committed to the view, ordered by ID, after optional ID to support keyset pagination.
func (v View[Data, Metadata, Patch]) List(ctx context.Context, after *identifier.Identifier) ([]identifier.Identifier, errors.E) {
	arguments := []any{
		v.name,
	}
	afterCondition := ""
	if after != nil {
		arguments = append(arguments, after.String())
		// We want to make sure that after value really exists.
		afterCondition = `WHERE EXISTS (SELECT 1 FROM "viewPath" JOIN "committedValues" USING ("view") WHERE "id"=$2) AND "id">$2`
	}
	var values []identifier.Identifier
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		values = make([]identifier.Identifier, 0, MaxPageLength)

		rows, err := tx.Query(ctx, `
			WITH "viewPath" AS (
				SELECT UNNEST("path") AS "view" FROM "currentViews" JOIN "views" USING ("view", "revision")
					WHERE "currentViews"."name"=$1
			)
			SELECT DISTINCT "id"
				FROM "viewPath" JOIN "committedValues" USING ("view")
				`+afterCondition+`
				-- We order by "id" to enable keyset pagination.
				ORDER BY "id"
				LIMIT `+maxPageLengthStr, arguments...)
		if err != nil {
			return internal.WithPgxError(err)
		}
		var i string
		_, err = pgx.ForEachRow(rows, []any{&i}, func() error {
			values = append(values, identifier.MustFromString(i))
			return nil
		})
		if err != nil {
			return internal.WithPgxError(err)
		}
		if len(values) == 0 {
			// TODO: Is there a better way to check without doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "currentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrViewNotFound)
			}
			if after != nil {
				err = tx.QueryRow(ctx, `SELECT EXISTS (
					WITH "viewPath" AS (
						SELECT UNNEST("path") AS "view" FROM "currentViews" JOIN "views" USING ("view", "revision")
							WHERE "currentViews"."name"=$1
					)
					SELECT 1 FROM "viewPath" JOIN "committedValues" USING ("view") WHERE "id"=$2
				)`, arguments...).Scan(&exists)
				if err != nil {
					return internal.WithPgxError(err)
				} else if !exists {
					return errors.WithStack(ErrValueNotFound)
				}
			}
			// There is nothing wrong with having no values.
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = v.name
		if after != nil {
			details["after"] = after.String()
		}
	}
	return values, errE
}

// Changes returns up to MaxPageLength changesets for the value committed to the view, ordered by first depth and then
// changeset ID, after optional changeset ID to support keyset pagination.
func (v View[Data, Metadata, Patch]) Changes(ctx context.Context, id identifier.Identifier, after *identifier.Identifier) ([]identifier.Identifier, errors.E) {
	if after != nil {
		return v.changesAfter(ctx, id, *after)
	}

	return v.changesInitial(ctx, id)
}

func (v View[Data, Metadata, Patch]) changesInitial(ctx context.Context, id identifier.Identifier) ([]identifier.Identifier, errors.E) {
	arguments := []any{
		v.name, id.String(),
	}
	var changesets []identifier.Identifier
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		changesets = make([]identifier.Identifier, 0, MaxPageLength)

		rows, err := tx.Query(ctx, `
			WITH "viewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.* FROM "currentViews" JOIN "views" USING ("view", "revision"), UNNEST("path") WITH ORDINALITY AS p("view", "depth")
					WHERE "currentViews"."name"=$1
			), "valueView" AS (
				SELECT "view"
					FROM "viewPath" JOIN "committedValues" USING ("view")
					WHERE "id"=$2
					-- We search views for the value in path order. This means that some later (ancestor)
					-- view might have a newer value version, but we still use the one which is explicitly
					-- committed to an earlier view.
					ORDER BY "viewPath"."depth" ASC
					-- We want only the first view with the value.
					LIMIT 1
			), "distinctChangesets" AS (
				SELECT DISTINCT ON ("changeset") "changeset", "depth"
				FROM
					-- This gives us changesets for the value's view.
					"valueView" JOIN "committedValues" USING ("view")
				WHERE "id"=$2
				ORDER BY
					-- We order by "changeset" first to be able to do DISTINCT ON.
					"changeset",
					-- Then we order in the order of graph traversal.
					"depth" ASC
			)
			SELECT "changeset"
				FROM "distinctChangesets"
				-- We return distinct "changeset" in the order of graph traversal.
				ORDER BY "depth" ASC
				LIMIT `+maxPageLengthStr, arguments...)
		if err != nil {
			return internal.WithPgxError(err)
		}
		var i string
		_, err = pgx.ForEachRow(rows, []any{&i}, func() error {
			changesets = append(changesets, identifier.MustFromString(i))
			return nil
		})
		if err != nil {
			return internal.WithPgxError(err)
		}
		if len(changesets) == 0 {
			// TODO: Is there a better way to check without doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "currentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrViewNotFound)
			}
			// There should be at least one change if value exists, the change inserting the value.
			return errors.WithStack(ErrValueNotFound)
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = v.name
		details["id"] = id.String()
	}
	return changesets, errE
}

func (v View[Data, Metadata, Patch]) changesAfter(ctx context.Context, id, after identifier.Identifier) ([]identifier.Identifier, errors.E) {
	arguments := []any{
		v.name, id.String(), after.String(),
	}
	var changesets []identifier.Identifier
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		changesets = make([]identifier.Identifier, 0, MaxPageLength)

		rows, err := tx.Query(ctx, `
			WITH "viewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.* FROM "currentViews" JOIN "views" USING ("view", "revision"), UNNEST("path") WITH ORDINALITY AS p("view", "depth")
					WHERE "currentViews"."name"=$1
			), "valueView" AS (
				SELECT "view"
					FROM "viewPath" JOIN "committedValues" USING ("view")
					WHERE "id"=$2
					-- We search views for the value in path order. This means that some later (ancestor)
					-- view might have a newer value version, but we still use the one which is explicitly
					-- committed to an earlier view.
					ORDER BY "viewPath"."depth" ASC
					-- We want only the first view with the value.
					LIMIT 1
			), "distinctChangesets" AS (
				SELECT DISTINCT ON ("changeset") "changeset", "depth"
				FROM
					-- This gives us changesets for the value's view.
					"valueView" JOIN "committedValues" USING ("view")
				WHERE "id"=$2
				ORDER BY
					-- We order by "changeset" first to be able to do DISTINCT ON.
					"changeset",
					-- Then we order in the order of graph traversal.
					"depth" ASC
			), "changesetDepth" AS (
				-- This should return at most one row.
				SELECT "depth" FROM "distinctChangesets" WHERE "changeset"=$3
			)
			SELECT "changeset"
				FROM "distinctChangesets", "changesetDepth"
				WHERE (
						-- Or a changeset is deeper than the after changeset.
						"distinctChangesets"."depth">"changesetDepth"."depth"
					) OR (
						-- Or a changeset is at the same depth than the after changeset,
						-- but it is sorted later.
						"distinctChangesets"."depth"="changesetDepth"."depth"
						AND "changeset">$3
					)
				ORDER BY
					-- We return distinct "changeset" in the order of graph traversal.
					"distinctChangesets"."depth" ASC,
					-- If there are multiple changesets at the same depth,
					-- we order by "id" to enable keyset pagination at the depth.
					"changeset" ASC
		`, arguments...)
		if err != nil {
			return internal.WithPgxError(err)
		}
		var i string
		_, err = pgx.ForEachRow(rows, []any{&i}, func() error {
			changesets = append(changesets, identifier.MustFromString(i))
			return nil
		})
		if err != nil {
			return internal.WithPgxError(err)
		}
		if len(changesets) == 0 {
			// TODO: Is there a better way to check without doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "currentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrViewNotFound)
			}
			err = tx.QueryRow(ctx, `
				SELECT EXISTS (
					WITH "viewPath" AS (
						SELECT UNNEST("path") AS "view" FROM "currentViews" JOIN "views" USING ("view", "revision")
							WHERE "currentViews"."name"=$1
					)
					SELECT 1 FROM "viewPath" JOIN "committedValues" USING ("view") WHERE "id"=$2
				)
			`, v.name, id.String()).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrValueNotFound)
			}
			err = tx.QueryRow(ctx, `
				SELECT EXISTS (
					WITH "viewPath" AS (
						-- We care about order of views so we annotate views in the path with view's index.
						SELECT p.* FROM "currentViews" JOIN "views" USING ("view", "revision"), UNNEST("path") WITH ORDINALITY AS p("view", "depth")
							WHERE "currentViews"."name"=$1
					), "valueView" AS (
						SELECT "view"
							FROM "viewPath" JOIN "committedValues" USING ("view")
							WHERE "id"=$2
							-- We search views for the value in path order. This means that some later (ancestor)
							-- view might have a newer value version, but we still use the one which is explicitly
							-- committed to an earlier view.
							ORDER BY "viewPath"."depth" ASC
							-- We want only the first view with the value.
							LIMIT 1
					)
					SELECT 1 FROM "valueView" JOIN "committedValues" USING ("view") WHERE "id"=$2 AND "changeset"=$3
				)
			`, arguments...).Scan(&exists)
			if err != nil {
				return internal.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrChangesetNotFound)
			}
			// There is nothing wrong with having no changes anymore for valid value ID and after a valid after changeset.
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = v.name
		details["id"] = id.String()
		details["after"] = after.String()
	}
	return changesets, errE
}

// TODO: Add a method which returns a requested change in full, including the patch and that it does not return an error if the change is for deletion.
//       Maybe Changeset should have Get which returns Change (without validating anything) which can then have methods to return different things.
//       Add to View.Get docstring that to get values of any changeset, you should then go through Changeset and not View.

// TODO: Support also name-less views (like the View but has to store view ID instead).

// TODO: Allow adding a name to an existing view.

// TODO: Allow views to start remove a value so that the value from the parent view is again automatically available.
//       For example, parent view might have resolved issues in its version of the value and the author of the current view
//       might not want to have an explicit locked version anymore as they are satisfied with the parent version now.

// Create creates a new view with the current view as its parent.
func (v View[Data, Metadata, Patch]) Create(ctx context.Context, name string, metadata Metadata) (View[Data, Metadata, Patch], errors.E) {
	arguments := []any{
		identifier.New().String(), name, metadata, v.name,
	}
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		res, err := tx.Exec(ctx, `
			INSERT INTO "views" SELECT $1, 1, $2, array_prepend($1, "path"), $3
				FROM "currentViews" JOIN "views" USING ("view", "revision")
				WHERE "currentViews"."name"=$4;
		`, arguments...)
		if err != nil {
			errE := internal.WithPgxError(err)
			var pgError *pgconn.PgError
			if errors.As(err, &pgError) {
				switch pgError.Code { //nolint:gocritic
				case internal.ErrorCodeUniqueViolation:
					return errors.WrapWith(errE, ErrConflict)
				}
			}
			return errE
		}
		if res.RowsAffected() == 0 {
			return errors.WithStack(ErrViewNotFound)
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = v.name
		details["name"] = name
		return View[Data, Metadata, Patch]{}, errE //nolint:exhaustruct
	}
	return View[Data, Metadata, Patch]{
		name:  name,
		store: v.store,
	}, nil
}

// Release releases (removes) the name of the view.
func (v View[Data, Metadata, Patch]) Release(ctx context.Context, metadata Metadata) errors.E {
	arguments := []any{
		v.name, metadata,
	}
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		res, err := tx.Exec(ctx, `
			INSERT INTO "views" SELECT "view", "revision"+1, NULL, "path", $2
				FROM "currentViews" JOIN "views" USING ("view", "revision")
				WHERE "currentViews"."name"=$1;
		`, arguments...)
		if err != nil {
			return internal.WithPgxError(err)
		}
		if res.RowsAffected() == 0 {
			return errors.WithStack(ErrViewNotFound)
		}
		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["view"] = v.name
	}
	return errE
}
