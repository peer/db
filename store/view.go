package store

import (
	"context"

	"github.com/jackc/pgx/v5"
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
	errE = changeset.Commit(ctx, metadata)
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
	errE = changeset.Commit(ctx, metadata)
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
	errE = changeset.Commit(ctx, metadata)
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
	errE = changeset.Commit(ctx, metadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE //nolint:exhaustruct
	}
	return version, nil
}

func (v *View[Data, Metadata, Patch]) GetCurrent(ctx context.Context, id identifier.Identifier) (Data, Metadata, Version, errors.E) { //nolint:ireturn
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
			WITH "currentViewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.* FROM "currentViews", "views", UNNEST("path") WITH ORDINALITY AS p("id", "depth")
					WHERE "currentViews"."name"=$1
					AND "currentViews"."id"="views"."id"
					AND "currentViews"."revision"="views"."revision"
			), "currentViewChangesets" AS (
				SELECT "changeset", "depth" FROM "committedChangesets", "currentViewPath" WHERE "committedChangesets"."view"="currentViewPath"."id"
			), "parentChangesets" AS (
				SELECT UNNEST("parentChangesets") AS "changeset" FROM "currentChanges", "changes"
					WHERE "currentChanges"."id"=$2
					AND "currentChanges"."changeset" IN (SELECT "changeset" FROM "currentViewChangesets")
					AND "currentChanges"."id"="changes"."id"
					AND "currentChanges"."changeset"="changes"."changeset"
					AND "currentChanges"."revision"="changes"."revision"
			)
			SELECT "currentChanges"."changeset", "currentChanges"."revision", "data", "data" IS NULL, "metadata"
				FROM "currentChanges", "changes", "currentViewChangesets"
					WHERE "currentChanges"."id"=$2
					AND "currentChanges"."id"="changes"."id"
					AND "currentChanges"."changeset"="changes"."changeset"
					AND "currentChanges"."revision"="changes"."revision"
					-- We collect all parent changesets for the object and make sure we do not select them.
					AND "currentChanges"."changeset" IN (SELECT "changeset" FROM "currentViewChangesets" EXCEPT SELECT * FROM "parentChangesets")
				-- It is important to search changesets in order in which views are listed in the path.
				-- There might be newer changesets for object ID in ancestor views, but younger views have
				-- to be explicitly rebased to include those newer changesets and until then we ignore them.
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
			// We return an error because this method is asking for current version of the object
			// but the object does not exist anymore. Other returned values are valid though.
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
				WITH "currentViewPath" AS (
					-- We do not care about order of views here because we have en explicit version we are searching for.
					SELECT p.* FROM "currentViews", "views", UNNEST("path") AS p("id")
						WHERE "currentViews"."name"=$1
						AND "currentViews"."id"="views"."id"
						AND "currentViews"."revision"="views"."revision"
				), "currentViewChangesets" AS (
					SELECT "changeset" FROM "committedChangesets", "currentViewPath" WHERE "committedChangesets"."view"="currentViewPath"."id"
				)
				SELECT "data", "data" IS NULL, "metadata" FROM "changes", "currentViewChangesets"
					WHERE "id"=$2
					AND "changes"."changeset"=$3
					AND "revision"=$4
					-- We require the object at given version has been committed to the view
					-- which we check by checking that version's changelog is among view's changelogs.
					AND "changes"."changeset"="currentViewChangesets"."changeset"
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
			// We return an error because this method is asking for a particular version of the object
			// but the object does not exist anymore at this version. Other returned values are valid though.
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
