package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

type View[Data, Metadata, Patch any] struct {
	Name string

	store *Store[Data, Metadata, Patch]
}

func (v *View[Data, Metadata, Patch]) Insert(ctx context.Context, id identifier.Identifier, value Data, metadata Metadata) (_ Version, errE errors.E) {
	changeset, errE := v.Begin(ctx)
	if errE != nil {
		return Version{}, errE
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Insert(ctx, id, value, metadata)
	if errE != nil {
		return Version{}, errE
	}
	errE = changeset.Commit(ctx, metadata)
	if errE != nil {
		return Version{}, errE
	}
	return version, nil
}

func (v *View[Data, Metadata, Patch]) Update(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata,
) (Version, errors.E) {
	changeset, errE := v.Begin(ctx)
	if errE != nil {
		return Version{}, errE
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Update(ctx, id, parentChangeset, value, patch, metadata)
	if errE != nil {
		return Version{}, errE
	}
	errE = changeset.Commit(ctx, metadata)
	if errE != nil {
		return Version{}, errE
	}
	return version, nil
}

func (v *View[Data, Metadata, Patch]) Delete(ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata) (Version, errors.E) {
	changeset, errE := v.Begin(ctx)
	if errE != nil {
		return Version{}, errE
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Delete(ctx, id, parentChangeset, metadata)
	if errE != nil {
		return Version{}, errE
	}
	errE = changeset.Commit(ctx, metadata)
	if errE != nil {
		return Version{}, errE
	}
	return version, nil
}

func (v *View[Data, Metadata, Patch]) GetCurrent(ctx context.Context, id identifier.Identifier) (Data, Metadata, Version, errors.E) {
	var data Data
	var metadata Metadata
	var version Version
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		var changeset string
		var revision int64
		err := tx.QueryRow(
			ctx, `
			WITH "currentViewPath" AS (
				SELECT p.* FROM "currentViews", UNNEST("path") WITH ORDINALITY AS p("id", "depth") WHERE "name"=$1
			), "currentViewChangesets" AS (
				SELECT "changeset", "depth" FROM "viewChangesets", "currentViewPath" WHERE "viewChangesets"."id"="currentViewPath"."id"
			)
			SELECT "currentChanges"."changeset", "revision", "data", "metadata" FROM "currentChanges", "currentViewChangesets" WHERE
				"id"=$2 AND "currentChanges"."changeset"="currentViewChangesets"."changeset"
				ORDER BY "depth" ASC
				LIMIT 1
		`,
			v.Name, id.String(),
		).Scan(&changeset, &revision, &data, &metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				errE = errors.WrapWith(errE, ErrValueNotFound)
			}
			details := errors.Details(errE)
			details["view"] = v.Name
			details["id"] = id.String()
			return errE
		}
		// TODO: Return ErrValueNotFound if "data" is NULL in returned row.
		version.Changeset = identifier.MustFromString(changeset)
		version.Revision = revision
		return nil
	})
	return data, metadata, version, errE
}

func (v *View[Data, Metadata, Patch]) Get(ctx context.Context, id identifier.Identifier, version Version) (Data, Metadata, errors.E) {
	var data Data
	var metadata Metadata
	errE := internal.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		err := tx.QueryRow(
			ctx, `
				WITH "currentViewPath" AS (
					SELECT p.* FROM "currentViews", UNNEST("path") AS p("id") WHERE "name"=$1
				), "currentViewChangesets" AS (
					SELECT "changeset" FROM "viewChangesets", "currentViewPath" WHERE "viewChangesets"."id"="currentViewPath"."id"
				)
				SELECT "data", "metadata" FROM "changes", "currentViewChangesets" WHERE
					"id"=$2 AND "changes"."changeset"=$3 AND "revision"=$4 AND "changes"."changeset"="currentViewChangesets"."changeset"
			`,
			v.Name, id.String(), version.Changeset.String(), version.Revision,
		).Scan(&data, &metadata)
		if err != nil {
			errE := internal.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				errE = errors.WrapWith(errE, ErrValueNotFound)
			}
			details := errors.Details(errE)
			details["view"] = v.Name
			details["id"] = id.String()
			details["changeset"] = version.Changeset.String()
			details["revision"] = version.Revision
			return errE
		}
		// TODO: Return ErrValueDeleted if "data" is NULL in returned row.
		return nil
	})
	return data, metadata, errE
}

func (v *View[Data, Metadata, Patch]) Changeset(ctx context.Context, id identifier.Identifier) (Changeset[Data, Metadata, Patch], errors.E) {
	return Changeset[Data, Metadata, Patch]{
		Identifier: id,
		view:       v,
	}, nil
}

func (v *View[Data, Metadata, Patch]) Begin(ctx context.Context) (Changeset[Data, Metadata, Patch], errors.E) {
	return v.Changeset(ctx, identifier.New())
}
