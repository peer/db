package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

// View is not a snapshot of the database but a dynamic named view of data to operate on.
type View[Data, Metadata, Patch any] struct {
	name  string
	store *Store[Data, Metadata, Patch]
}

func (v *View[Data, Metadata, Patch]) Name() string {
	return v.name
}

func (v *View[Data, Metadata, Patch]) Store() *Store[Data, Metadata, Patch] {
	return v.store
}

func (v *View[Data, Metadata, Patch]) Insert( //nolint:nonamedreturns
	ctx context.Context, id identifier.Identifier, value Data, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.Begin(ctx)
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
	_, errE = changeset.Commit(ctx, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

func (v *View[Data, Metadata, Patch]) Replace( //nolint:nonamedreturns
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.Begin(ctx)
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
	_, errE = changeset.Commit(ctx, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

func (v *View[Data, Metadata, Patch]) Update( //nolint:nonamedreturns
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.Begin(ctx)
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
	_, errE = changeset.Commit(ctx, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

func (v *View[Data, Metadata, Patch]) Merge( //nolint:nonamedreturns
	ctx context.Context, id identifier.Identifier, parentChangesets []identifier.Identifier, value Data, patches []Patch, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.Begin(ctx)
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
	_, errE = changeset.Commit(ctx, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

func (v *View[Data, Metadata, Patch]) Delete( //nolint:nonamedreturns
	ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata,
) (_ Version, errE errors.E) {
	changeset, errE := v.Begin(ctx)
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
	_, errE = changeset.Commit(ctx, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

func (v *View[Data, Metadata, Patch]) GetLatest(ctx context.Context, id identifier.Identifier) (Data, Metadata, Version, errors.E) { //nolint:ireturn
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
			)
			SELECT "changeset", "revision", "data", "data" IS NULL, "metadata"
				FROM "viewPath"
					-- This gives us the latest changesets for each view for each value.
					JOIN "committedValues" USING ("view")
					JOIN (
						-- This gives us only current revisions. And corresponding data and metadata.
						"currentChanges" JOIN "changes" USING ("changeset", "id", "revision")
					) USING ("changeset", "id")
				WHERE "id"=$2
				-- We search views for the value in path order. This means that some later (ancestor)
				-- view might have a newer value version, but we still use the one which is explicitly
				-- committed to an earlier view.
				ORDER BY "depth" ASC
				-- We care only about the first matching changeset.
				LIMIT 1
		`, arguments...).Scan(&changeset, &revision, &data, &dataIsNull, &metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// TODO: Is there a better way to check without doing another query?
				var exists bool
				err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "currentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
				if err != nil {
					return errors.Join(errE, err)
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

func (v *View[Data, Metadata, Patch]) Get(ctx context.Context, id identifier.Identifier, version Version) (Data, Metadata, errors.E) { //nolint:ireturn
	arguments := []any{
		v.name, id.String(), version.Changeset.String(), version.Revision,
	}
	var data Data
	var metadata Metadata
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		var dataIsNull bool
		err := tx.QueryRow(ctx, `
			WITH "viewPath" AS (
				SELECT UNNEST("path") AS "view" FROM "currentViews" JOIN "views" USING ("view", "revision")
					WHERE "currentViews"."name"=$1
				), "viewChangesets" AS (
					SELECT "changeset" FROM "currentCommittedChangesets" JOIN "viewPath" USING ("view")
				)
				-- We require the value at given version has been committed to the view (or its ancestors)
				-- which we check by checking that version's changeset is among view's changesets.
				SELECT "data", "data" IS NULL, "metadata"
					FROM "changes" JOIN "viewChangesets" USING ("changeset")
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
					return errors.Join(errE, err)
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

// TODO: Add a method which returns a requested change in full, including the patch and that it does not return an error if the change is for deletion.

func (v *View[Data, Metadata, Patch]) Changeset(_ context.Context, id identifier.Identifier) (Changeset[Data, Metadata, Patch], errors.E) {
	// We do not care if the view exists at this point. It all
	// depends what we will be doing with it and we do checks then.
	return Changeset[Data, Metadata, Patch]{
		Identifier: id,
		view:       v,
	}, nil
}

func (v *View[Data, Metadata, Patch]) Begin(ctx context.Context) (Changeset[Data, Metadata, Patch], errors.E) {
	return v.Changeset(ctx, identifier.New())
}

// TODO: Support also name-less views (but the View has to store view ID instead).

// Create creates a new view based on the current view.
func (v *View[Data, Metadata, Patch]) Create(ctx context.Context, name string, metadata Metadata) (View[Data, Metadata, Patch], errors.E) {
	arguments := []any{
		identifier.New().String(), name, metadata, v.name,
	}
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		res, err := tx.Exec(ctx, `
			INSERT INTO "views" SELECT $1, 1, $2, array_prepend($2, "path"), $3
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
func (v *View[Data, Metadata, Patch]) Release(ctx context.Context, metadata Metadata) errors.E {
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
