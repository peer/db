package store

import (
	"context"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	// MaxPageLength is the maximum number of items that can be returned in a single page.
	MaxPageLength    = 5000
	maxPageLengthStr = "5000"
)

// ChangesetID is an interface for a changeset ID from commit metadata.
//
// It is used with auto-committing methods on View and Store to control
// the ID of the changeset made.
type ChangesetID interface {
	ChangesetID() identifier.Identifier
}

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
type View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any] struct {
	name  string
	store *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
}

// Name of this named view.
//
// Name is unique across all named views at every point in time. A view can release a
// name and another view can then be created with that name.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Name() string {
	return v.name
}

func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) String() string {
	return v.name
}

// Store returns the Store associated with the view.
//
// It can return nil if Store is not associated with the view.
// You can use WithStore to associate it.
//
//nolint:lll
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Store() *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch] {
	return v.store
}

// WithStore returns a new View object associated with the given Store.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) WithStore(
	ctx context.Context, store *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch],
) (View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	return store.View(ctx, v.name)
}

// Commit commits a changeset to the view.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Commit(
	ctx context.Context, changeset Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], metadata CommitMetadata,
) ([]Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	return changeset.Commit(ctx, v, metadata)
}

// Insert auto-commits the insert change into the view.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Insert( //nolint:nonamedreturns
	ctx context.Context, id identifier.Identifier, value Data, metadata Metadata, commitMetadata CommitMetadata,
) (_ Version, errE errors.E) {
	var changeset Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
	if c, ok := any(commitMetadata).(ChangesetID); ok {
		changeset, errE = v.store.Changeset(ctx, c.ChangesetID())
	} else {
		changeset, errE = v.store.Begin(ctx)
	}
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
	_, errE = changeset.Commit(ctx, v, commitMetadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE
	}
	return version, nil
}

// TODO: Support replacing/updating/merging the value as Reader interface value instead of whole Data in memory.

// Replace auto-commits the replace change into the view.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Replace( //nolint:nonamedreturns
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, metadata Metadata, commitMetadata CommitMetadata,
) (_ Version, errE errors.E) {
	var changeset Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
	if c, ok := any(commitMetadata).(ChangesetID); ok {
		changeset, errE = v.store.Changeset(ctx, c.ChangesetID())
	} else {
		changeset, errE = v.store.Begin(ctx)
	}
	if errE != nil {
		return Version{}, errE
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Replace(ctx, id, parentChangeset, value, metadata)
	if errE != nil {
		return Version{}, errE
	}
	_, errE = changeset.Commit(ctx, v, commitMetadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE
	}
	return version, nil
}

// Update auto-commits the update change into the view.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Update( //nolint:nonamedreturns
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata, commitMetadata CommitMetadata,
) (_ Version, errE errors.E) {
	var changeset Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
	if c, ok := any(commitMetadata).(ChangesetID); ok {
		changeset, errE = v.store.Changeset(ctx, c.ChangesetID())
	} else {
		changeset, errE = v.store.Begin(ctx)
	}
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
	_, errE = changeset.Commit(ctx, v, commitMetadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE
	}
	return version, nil
}

// Merge auto-commits the merge change into the view.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Merge( //nolint:nonamedreturns
	ctx context.Context, id identifier.Identifier, parentChangesets []identifier.Identifier, value Data, patches []Patch, metadata Metadata, commitMetadata CommitMetadata,
) (_ Version, errE errors.E) {
	var changeset Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
	if c, ok := any(commitMetadata).(ChangesetID); ok {
		changeset, errE = v.store.Changeset(ctx, c.ChangesetID())
	} else {
		changeset, errE = v.store.Begin(ctx)
	}
	if errE != nil {
		return Version{}, errE
	}
	defer func() {
		errE = errors.Join(errE, changeset.Rollback(ctx))
	}()
	version, errE := changeset.Merge(ctx, id, parentChangesets, value, patches, metadata)
	if errE != nil {
		return Version{}, errE
	}
	_, errE = changeset.Commit(ctx, v, commitMetadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE
	}
	return version, nil
}

// Delete auto-commits the delete change into the view.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Delete( //nolint:nonamedreturns
	ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata, commitMetadata CommitMetadata,
) (_ Version, errE errors.E) {
	var changeset Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
	if c, ok := any(commitMetadata).(ChangesetID); ok {
		changeset, errE = v.store.Changeset(ctx, c.ChangesetID())
	} else {
		changeset, errE = v.store.Begin(ctx)
	}
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
	_, errE = changeset.Commit(ctx, v, commitMetadata)
	if errE != nil {
		errors.Details(errE)["id"] = id.String()
		return Version{}, errE
	}
	return version, nil
}

// TODO: Support getting the value as ReadSeekCloser interface value instead of whole Data in memory.

// GetLatest returns the latest committed version of the value for the view.
//
// The latest committed version is not the latest based on the time it was made,
// but that it is the latest in the graph of committed changes to the value
// (i.e., no other change for the value has this version of the value as the parent
// version). Each view can have only one latest committed version for each value.
//
// A view might not have an explicitly committed version of a given value, but its
// ancestor views might. In that case the value is searched for in the ancestry order,
// first the parent view. This means that some later (older) view might have a
// newer value version, but GetLatest still returns the value version which is
// explicitly committed to an earlier (younger) view, i.e., the view shadows values
// and value versions from the parent view for those explicitly committed to the view.
//
// If value has been deleted, ErrValueDeleted error is returned, but other returned
// values are valid as well.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) GetLatest( //nolint:ireturn
	ctx context.Context, id identifier.Identifier,
) (Data, Metadata, Version, []Version, errors.E) {
	arguments := []any{
		v.name, id.String(),
	}
	var data Data
	var metadata Metadata
	var version Version
	var parentChangesets []Version
	errE := internalStore.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		data = *new(Data)
		metadata = *new(Metadata)
		version = Version{}
		parentChangesets = nil

		var changeset string
		var revision int64
		var dataIsNull bool
		var parentChangesetsString []string

		err := tx.QueryRow(ctx, `
			WITH "viewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.* FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision"),
						UNNEST("path") WITH ORDINALITY AS p("view", "depth")
					WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1
			), "valueView" AS (
				SELECT "view"
					FROM "viewPath" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
					WHERE "id"=$2
					-- We search views for the value in path order. This means that some later (ancestor)
					-- view might have a newer value version, but we still use the one which is explicitly
					-- committed to an earlier view.
					ORDER BY "viewPath"."depth" ASC
					-- We want only the first view with the value.
					LIMIT 1
			)
			SELECT "changeset", "revision", "data", "data" IS NULL, "metadata", "parentChangesets"
				FROM
					-- This gives us changesets for the value's view.
					"valueView" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
					JOIN (
						-- This gives us current revisions. And corresponding data and metadata.
						"`+v.store.Prefix+`CurrentChanges" JOIN "`+v.store.Prefix+`Changes" USING ("changeset", "id", "revision")
					) USING ("changeset", "id")
				WHERE "id"=$2
					-- We want the latest explicitly committed version of the value.
					-- We know there can be at most one row because we have a CONSTRAINT to ensure that.
					AND "depth"=0
		`, arguments...).Scan(&changeset, &revision, &data, &dataIsNull, &metadata, &parentChangesetsString)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// TODO: Is there a better way to check without doing another query?
				var exists bool
				err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+v.store.Prefix+`CurrentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
				if err != nil {
					return errors.Join(errE, internalStore.WithPgxError(err))
				} else if !exists {
					return errors.WrapWith(errE, ErrViewNotFound)
				}
				return errors.WrapWith(errE, ErrValueNotFound)
			}
			return errE
		}
		version.Changeset = identifier.String(changeset)
		version.Revision = revision
		for _, s := range parentChangesetsString {
			parentChangesets = append(parentChangesets, Version{Changeset: identifier.String(s), Revision: 0})
		}
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
	return data, metadata, version, parentChangesets, errE
}

// Get returns the value at a given version for the view.
//
// Get first searches for the view (including ancestor views) which has the value
// and then returns the value only for versions available for that view.
// This means that some later (older) view might have a
// newer value version, but Get will not return it even if asked for if there is an
// older version explicitly committed to an earlier (younger) view, i.e., the view
// shadows values and value versions from the parent view for those explicitly
// committed to the view.
//
// If version has Revision set to 0, the latest revision for the given changeset is returned.
//
// If value has been deleted at a given version, ErrValueDeleted error is returned,
// but other returned values are valid as well.
//
//nolint:ireturn
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Get(
	ctx context.Context, id identifier.Identifier, version Version,
) (Data, Metadata, Version, []Version, errors.E) {
	arguments := []any{
		v.name, id.String(), version.Changeset.String(),
	}
	revisionCondition := ""
	if version.Revision > 0 {
		arguments = append(arguments, version.Revision)
		revisionCondition = `	AND "revision"=$4`
	} else {
		revisionCondition = `ORDER BY "revision" DESC LIMIT 1`
	}
	var data Data
	var metadata Metadata
	var resolved Version
	var parentChangesets []Version
	errE := internalStore.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		data = *new(Data)
		metadata = *new(Metadata)
		resolved = Version{}
		parentChangesets = nil

		var dataIsNull bool
		var resolvedRevision int64
		var parentChangesetsString []string
		err := tx.QueryRow(ctx, `
			WITH "viewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.* FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision"), UNNEST("path") WITH ORDINALITY AS p("view", "depth")
					WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1
			), "valueView" AS (
				SELECT "view"
					FROM "viewPath" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
					WHERE "id"=$2
					-- We search views for the value in path order. This means that some later (ancestor)
					-- view might have a newer value version, but we still use the one which is explicitly
					-- committed to an earlier view.
					ORDER BY "viewPath"."depth" ASC
					-- We want only the first view with the value.
					LIMIT 1
			)
			SELECT "revision", "data", "data" IS NULL, "metadata", "parentChangesets"
				FROM
					-- This gives us changesets for the value's view.
					"valueView" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
					-- This gives us corresponding data and metadata.
					JOIN "`+v.store.Prefix+`Changes" USING ("changeset", "id")
				WHERE "id"=$2
					AND "changeset"=$3
				`+revisionCondition,
			arguments...).Scan(&resolvedRevision, &data, &dataIsNull, &metadata, &parentChangesetsString)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if errors.Is(err, pgx.ErrNoRows) {
				// TODO: Is there a better way to check without doing another query?
				var exists bool
				err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+v.store.Prefix+`CurrentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
				if err != nil {
					return errors.Join(errE, internalStore.WithPgxError(err))
				} else if !exists {
					return errors.WrapWith(errE, ErrViewNotFound)
				}
				return errors.WrapWith(errE, ErrValueNotFound)
			}
			return errE
		}
		resolved.Changeset = version.Changeset
		resolved.Revision = resolvedRevision
		for _, s := range parentChangesetsString {
			parentChangesets = append(parentChangesets, Version{Changeset: identifier.String(s), Revision: 0})
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
	return data, metadata, resolved, parentChangesets, errE
}

// List returns up to MaxPageLength value IDs committed to the view, ordered by ID, after optional ID, to support keyset pagination.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) List(
	ctx context.Context, after *identifier.Identifier,
) ([]identifier.Identifier, errors.E) {
	arguments := []any{
		v.name,
	}
	afterCondition := ""
	if after != nil {
		arguments = append(arguments, after.String())
		// We want to make sure that after value really exists.
		afterCondition = `WHERE EXISTS (SELECT 1 FROM "viewPath" JOIN "` + v.store.Prefix + `CommittedValues" USING ("view") WHERE "id"=$2) AND "id">$2`
	}
	var values []identifier.Identifier
	errE := internalStore.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		values = make([]identifier.Identifier, 0, MaxPageLength)

		rows, err := tx.Query(ctx, `
			WITH "viewPath" AS (
				SELECT UNNEST("path") AS "view" FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision")
					WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1
			)
			SELECT DISTINCT "id"
				FROM "viewPath" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
				`+afterCondition+`
				-- We order by "id" to enable keyset pagination.
				ORDER BY "id"
				LIMIT `+maxPageLengthStr, arguments...)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		var i string
		_, err = pgx.ForEachRow(rows, []any{&i}, func() error {
			values = append(values, identifier.String(i))
			return nil
		})
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		if len(values) == 0 {
			// TODO: Is there a better way to check without doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+v.store.Prefix+`CurrentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
			if err != nil {
				return internalStore.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrViewNotFound)
			}
			if after != nil {
				err = tx.QueryRow(ctx, `SELECT EXISTS (
					WITH "viewPath" AS (
						SELECT UNNEST("path") AS "view" FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision")
							WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1
					)
					SELECT 1 FROM "viewPath" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view") WHERE "id"=$2
				)`, arguments...).Scan(&exists)
				if err != nil {
					return internalStore.WithPgxError(err)
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

// Count returns the number of distinct values currently committed to the view,
// excluding values whose latest committed version has been deleted.
//
// For each value the latest committed version is determined the same way as in
// GetLatest: the closest ancestor view in the path that has the value at
// depth=0. If that latest version's data is NULL (the value has been deleted)
// the value is not counted.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Count(
	ctx context.Context,
) (int64, errors.E) {
	var count int64
	errE := internalStore.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		count = 0

		err := tx.QueryRow(ctx, `
			WITH "viewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.*
					FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision"),
						UNNEST("path") WITH ORDINALITY AS p("view", "depth")
					WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1
			), "latestPerId" AS (
				-- For each value pick the closest view in the path that has it at depth=0.
				SELECT DISTINCT ON ("id") "id", "changeset"
					FROM "viewPath" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
					WHERE "`+v.store.Prefix+`CommittedValues"."depth"=0
					ORDER BY "id", "viewPath"."depth" ASC
			)
			SELECT COUNT(*)
				FROM "latestPerId"
					JOIN (
						-- This gives us current revisions. And corresponding data and metadata.
						"`+v.store.Prefix+`CurrentChanges" JOIN "`+v.store.Prefix+`Changes" USING ("changeset", "id", "revision")
					) USING ("changeset", "id")
				WHERE "data" IS NOT NULL
		`, v.name).Scan(&count)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		if count == 0 {
			// TODO: Is there a better way to check without doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+v.store.Prefix+`CurrentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
			if err != nil {
				return internalStore.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrViewNotFound)
			}
			// There is nothing wrong with having no values.
		}
		return nil
	})
	if errE != nil {
		errors.Details(errE)["view"] = v.name
	}
	return count, errE
}

// Changes returns up to MaxPageLength changesets for the value committed to the view, ordered first by depth
// in increasing order (newest changes first) and then by changeset ID, after optional changeset ID, to
// support keyset pagination.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Changes(
	ctx context.Context, id identifier.Identifier, after *identifier.Identifier,
) ([]identifier.Identifier, errors.E) {
	if after != nil {
		return v.changesAfter(ctx, id, *after)
	}

	return v.changesInitial(ctx, id)
}

func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) changesInitial(
	ctx context.Context, id identifier.Identifier,
) ([]identifier.Identifier, errors.E) {
	arguments := []any{
		v.name, id.String(),
	}
	var changesets []identifier.Identifier
	errE := internalStore.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		changesets = make([]identifier.Identifier, 0, MaxPageLength)

		rows, err := tx.Query(ctx, `
			WITH "viewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.* FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision"),
						UNNEST("path") WITH ORDINALITY AS p("view", "depth")
					WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1
			), "valueView" AS (
				SELECT "view"
					FROM "viewPath" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
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
					"valueView" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
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
			return internalStore.WithPgxError(err)
		}
		var i string
		_, err = pgx.ForEachRow(rows, []any{&i}, func() error {
			changesets = append(changesets, identifier.String(i))
			return nil
		})
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		if len(changesets) == 0 {
			// TODO: Is there a better way to check without doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+v.store.Prefix+`CurrentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
			if err != nil {
				return internalStore.WithPgxError(err)
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

func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) changesAfter(
	ctx context.Context, id, after identifier.Identifier,
) ([]identifier.Identifier, errors.E) {
	arguments := []any{
		v.name, id.String(), after.String(),
	}
	var changesets []identifier.Identifier
	errE := internalStore.RetryTransaction(ctx, v.store.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		changesets = make([]identifier.Identifier, 0, MaxPageLength)

		rows, err := tx.Query(ctx, `
			WITH "viewPath" AS (
				-- We care about order of views so we annotate views in the path with view's index.
				SELECT p.* FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision"),
						UNNEST("path") WITH ORDINALITY AS p("view", "depth")
					WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1
			), "valueView" AS (
				SELECT "view"
					FROM "viewPath" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
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
					"valueView" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
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
				LIMIT `+maxPageLengthStr, arguments...)
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		var i string
		_, err = pgx.ForEachRow(rows, []any{&i}, func() error {
			changesets = append(changesets, identifier.String(i))
			return nil
		})
		if err != nil {
			return internalStore.WithPgxError(err)
		}
		if len(changesets) == 0 {
			// TODO: Is there a better way to check without doing another query?
			var exists bool
			err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM "`+v.store.Prefix+`CurrentViews" WHERE "name"=$1)`, v.name).Scan(&exists)
			if err != nil {
				return internalStore.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrViewNotFound)
			}
			err = tx.QueryRow(ctx, `
				SELECT EXISTS (
					WITH "viewPath" AS (
						SELECT UNNEST("path") AS "view" FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision")
							WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1
					)
					SELECT 1 FROM "viewPath" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view") WHERE "id"=$2
				)
			`, v.name, id.String()).Scan(&exists)
			if err != nil {
				return internalStore.WithPgxError(err)
			} else if !exists {
				return errors.WithStack(ErrValueNotFound)
			}
			err = tx.QueryRow(ctx, `
				SELECT EXISTS (
					WITH "viewPath" AS (
						-- We care about order of views so we annotate views in the path with view's index.
						SELECT p.* FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision"),
								UNNEST("path") WITH ORDINALITY AS p("view", "depth")
							WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1
					), "valueView" AS (
						SELECT "view"
							FROM "viewPath" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view")
							WHERE "id"=$2
							-- We search views for the value in path order. This means that some later (ancestor)
							-- view might have a newer value version, but we still use the one which is explicitly
							-- committed to an earlier view.
							ORDER BY "viewPath"."depth" ASC
							-- We want only the first view with the value.
							LIMIT 1
					)
					SELECT 1 FROM "valueView" JOIN "`+v.store.Prefix+`CommittedValues" USING ("view") WHERE "id"=$2 AND "changeset"=$3
				)
			`, arguments...).Scan(&exists)
			if err != nil {
				return internalStore.WithPgxError(err)
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

// TODO: Support also name-less views (like the View but has to store view ID instead).

// TODO: Allow adding a name to an existing view.

// TODO: Allow views to remove a value so that the value from the parent view is again automatically available.
//       For example, parent view might have resolved issues in its version of the value and the author of the current view
//       might not want to have an explicit locked version anymore as they are satisfied with the parent version now.

// Create creates a new view with the current view as its parent.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Create(
	ctx context.Context, name string, metadata CreateViewMetadata,
) (View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	arguments := []any{
		identifier.New().String(), name, metadata, v.name,
	}
	errE := internalStore.RetryTransaction(ctx, v.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		res, err := tx.Exec(ctx, `
			INSERT INTO "`+v.store.Prefix+`Views" SELECT $1, 1, $2, array_prepend($1, "path"), $3
				FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision")
				WHERE "`+v.store.Prefix+`CurrentViews"."name"=$4;
		`, arguments...)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code { //nolint:gocritic
				case pgerrcode.UniqueViolation:
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
		return View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{}, errE
	}
	return View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
		name:  name,
		store: v.store,
	}, nil
}

// Release releases (removes) the name of the view.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Release(ctx context.Context, metadata ReleaseViewMetadata) errors.E {
	arguments := []any{
		v.name, metadata,
	}
	errE := internalStore.RetryTransaction(ctx, v.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		res, err := tx.Exec(ctx, `
			INSERT INTO "`+v.store.Prefix+`Views" SELECT "view", "revision"+1, NULL, "path", $2
				FROM "`+v.store.Prefix+`CurrentViews" JOIN "`+v.store.Prefix+`Views" USING ("view", "revision")
				WHERE "`+v.store.Prefix+`CurrentViews"."name"=$1;
		`, arguments...)
		if err != nil {
			return internalStore.WithPgxError(err)
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

// UpdateExistingMetadata updates the metadata for an existing changeset revision
// by inserting a new revision that copies data and patches from the current revision
// but uses the provided metadata. The version must refer to the current (latest) revision.
func (v View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) UpdateExistingMetadata(
	ctx context.Context, id identifier.Identifier, version Version, metadata Metadata,
) (Version, errors.E) {
	var newVersion Version
	errE := internalStore.RetryTransaction(ctx, v.store.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		newVersion = Version{}

		patchesColumn := ""
		if v.store.patchesEnabled {
			patchesColumn = `, "patches"`
		}
		res, err := tx.Exec(ctx, `
			SELECT "`+v.store.Prefix+`ChangesetUpdateExisting"($1, $2, $3, "data", $4`+patchesColumn+`)
				FROM "`+v.store.Prefix+`Changes"
				WHERE "changeset"=$1 AND "id"=$2 AND "revision"=$3
		`, version.Changeset.String(), id.String(), version.Revision, metadata)
		if err != nil {
			errE := internalStore.WithPgxError(err)
			if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
				switch pgError.Code {
				case errorCodeChangesetNotFound:
					return errors.WrapWith(errE, ErrChangesetNotFound)
				case errorCodeRevisionMismatch:
					return errors.WrapWith(errE, ErrRevisionMismatch)
				}
			}
			return errE
		}
		if res.RowsAffected() == 0 {
			// If changeset or revision does not exist, the outer SELECT does not match anything and
			// it does not even call ChangesetUpdateExisting. We detect this case here.
			return errors.WithStack(ErrChangesetNotFound)
		}

		newVersion = Version{
			Changeset: version.Changeset,
			Revision:  version.Revision + 1,
		}

		return nil
	})
	if errE != nil {
		details := errors.Details(errE)
		details["id"] = id.String()
		details["changeset"] = version.Changeset.String()
		details["revision"] = version.Revision
	}
	return newVersion, errE
}
