package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

// None is a special type which can be used for Patch type parameter
// to configure the Store instance to not use nor store patches.
type None *struct{}

// MainView is the name of the main view.
const MainView = "main"

const (
	// Our PostgreSQL error codes.
	errorCodeNotAllowed        = "P1000"
	errorCodeAlreadyCommitted  = "P1001"
	errorCodeInUse             = "P1002"
	errorCodeParentInvalid     = "P1003"
	errorCodeViewNotFound      = "P1004"
	errorCodeChangesetNotFound = "P1005"
)

// CommittedChangeset represents a changeset committed to a view.
type CommittedChangeset[Data, Metadata, Patch any] struct {
	Changeset Changeset[Data, Metadata, Patch]
	View      View[Data, Metadata, Patch]
}

// WithStore returns a new CommittedChangeset object with
// changeset and view associated with the given Store.
func (c CommittedChangeset[Data, Metadata, Patch]) WithStore(
	ctx context.Context, store *Store[Data, Metadata, Patch],
) (CommittedChangeset[Data, Metadata, Patch], errors.E) {
	changeset, errE1 := c.Changeset.WithStore(ctx, store)
	view, errE2 := c.View.WithStore(ctx, store)
	return CommittedChangeset[Data, Metadata, Patch]{
		Changeset: changeset,
		View:      view,
	}, errors.Join(errE1, errE2)
}

// Store is a key-value store which preserves history of changes.
//
// For every change, a new value, its metadata, and optional forward
// patches are stored. Go types for them you configure with type parameters.
// You can use special None type to configure the Store instance to not
// use nor store patches.
type Store[Data, Metadata, Patch any] struct {
	// PostgreSQL schema used by this store.
	Schema string

	// A channel to which changesets are send when they are committed.
	// The changesets and view objects sent do not have an associated Store.
	//
	// The order in which they are sent is not necessary the order in which
	// they were committed. You should not rely on the order.
	Committed chan<- CommittedChangeset[Data, Metadata, Patch]

	// PostgreSQL column types to store data, metadata, and patches.
	// It should probably be one of the jsonb, bytea, or text.
	// Go types used for Store type parameters should be compatible with
	// column types chosen.
	DataType     string
	MetadataType string
	PatchType    string

	dbpool         *pgxpool.Pool
	patchesEnabled bool
}

func (s *Store[Data, Metadata, Patch]) tryCreateSchema(ctx context.Context, tx pgx.Tx) (bool, errors.E) {
	_, err := tx.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, s.Schema))
	if err != nil {
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) {
			switch pgError.Code {
			case internal.ErrorCodeUniqueViolation:
				return false, nil
			case internal.ErrorCodeDuplicateSchema:
				return false, nil
			}
		}
		return false, internal.WithPgxError(err)
	}
	return true, nil
}

// Init initializes the Store.
//
// It creates and configures the PostgreSQL schema if it does not yet exist.
func (s *Store[Data, Metadata, Patch]) Init(ctx context.Context, dbpool *pgxpool.Pool) (errE errors.E) { //nolint:nonamedreturns,maintidx
	if s.dbpool != nil {
		return errors.New("already initialized")
	}

	s.patchesEnabled = !isNoneType[Patch]()

	errE = internal.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		created, errE := s.tryCreateSchema(ctx, tx) //nolint:govet
		if errE != nil {
			return errE
		}

		// TODO: Use schema management/migration instead.
		if created {
			patches := ""
			patchesArgument := ""
			patchesValue := ""
			if s.patchesEnabled {
				patches = `
					-- Forward patches which bring parentChangesets versions of the value to
					-- this version of the value. If patches are available, the number of patches
					-- and their order must match that of parentChangesets. All patches have to
					-- end up with the equal value.
					"patches" ` + s.PatchType + `[] NOT NULL,
				`
				patchesArgument = ", _patches " + s.PatchType + "[]"
				patchesValue = ", _patches"
			}

			_, err := tx.Exec(ctx, `
				CREATE FUNCTION "doNotAllow"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						RAISE EXCEPTION 'not allowed' USING ERRCODE = '`+errorCodeNotAllowed+`';
					END;
				$$;

				-- "changes" table contains all changes to values.
				CREATE TABLE "changes" (
					-- ID of the changeset this change belongs to.
					"changeset" text STORAGE PLAIN COLLATE "C" NOT NULL,
					-- ID of the value.
					"id" text STORAGE PLAIN COLLATE "C" NOT NULL,
					-- Revision of this change.
					"revision" bigint NOT NULL,
					-- IDs of changesets this value has been changed the last before this change.
					-- The same changeset ID can happen to repeat when melding multiple values
					-- (parentIds is then set, too).
					"parentChangesets" text[] COLLATE "C" NOT NULL,
					-- Direct previous IDs of this value. Multiple if this change is melding multiple
					-- values (number of IDs and order matches parentChangesets). Only one ID if a new
					-- value is being forked from the existing one (in this case history is preserved,
					-- but a new identity is made). An empty array means that the ID has not changed.
					-- The same parent ID can happen to repeat when both merging and melding at the
					-- same time.
					"parentIds" text[] COLLATE "C" NOT NULL DEFAULT '{}',
					-- Data of the value at this version of the value.
					-- NULL if value has been deleted.
					"data" `+s.DataType+`,
					"metadata" `+s.MetadataType+` NOT NULL,
					`+patches+`
					PRIMARY KEY ("changeset", "id", "revision")
				);
				CREATE INDEX ON "changes" USING gin ("parentChangesets");
				CREATE FUNCTION "changesAfterInsertFunc"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						INSERT INTO "currentChanges"
							SELECT DISTINCT ON ("changeset", "id") "changeset", "id", "revision" FROM NEW_ROWS
								ORDER BY "changeset", "id", "revision" DESC
								ON CONFLICT ("changeset", "id") DO UPDATE
									SET "revision"=EXCLUDED."revision";
						RETURN NULL;
					END;
				$$;
				CREATE FUNCTION "changesAfterDeleteFunc"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						DELETE FROM "currentChanges" USING OLD_ROWS
							WHERE "currentChanges"."changeset"=OLD_ROWS."changeset"
								AND "currentChanges"."id"=OLD_ROWS."id";
						INSERT INTO "currentChanges"
							SELECT DISTINCT ON ("changeset", "id") "changeset", "id", "changes"."revision"
								FROM OLD_ROWS JOIN "changes" USING ("changeset", "id")
								ORDER BY "changeset", "id", "changes"."revision" DESC
								ON CONFLICT ("changeset", "id") DO UPDATE
									SET "revision"=EXCLUDED."revision";
						RETURN NULL;
					END;
				$$;
				CREATE TRIGGER "changesAfterInsert" AFTER INSERT ON "changes"
					REFERENCING NEW TABLE AS NEW_ROWS
					FOR EACH STATEMENT EXECUTE FUNCTION "changesAfterInsertFunc"();
				CREATE TRIGGER "changesAfterDelete" AFTER DELETE ON "changes"
					REFERENCING OLD TABLE AS OLD_ROWS
					FOR EACH STATEMENT EXECUTE FUNCTION "changesAfterDeleteFunc"();
				CREATE TRIGGER "changesNotAllowed" BEFORE UPDATE OR TRUNCATE ON "changes"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				-- "views" table contains all changes to views.
				CREATE TABLE "views" (
					-- ID of the view.
					"view" text STORAGE PLAIN COLLATE "C" NOT NULL,
					-- Revision of this view.
					"revision" bigint NOT NULL,
					-- Name of the view. Optional.
					"name" text,
					-- Path of view IDs starting with the current view, then the
					-- parent view, and then all further ancestors.
					"path" text[] COLLATE "C" NOT NULL,
					"metadata" `+s.MetadataType+` NOT NULL,
					PRIMARY KEY ("view", "revision"),
					-- We do not allow empty strings for names. Use NULL instead.
					-- This allows us to use UNIQUE constraint in "currentViews.
					CHECK ("name"<>'')
				);
				CREATE INDEX ON "views" USING btree ("name");
				CREATE FUNCTION "viewsAfterInsertFunc"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						INSERT INTO "currentViews"
							SELECT DISTINCT ON ("view") "view", "revision", "name" FROM NEW_ROWS
								ORDER BY "view", "revision" DESC
								ON CONFLICT ("view") DO UPDATE
									SET "revision"=EXCLUDED."revision", "name"=EXCLUDED."name";
						RETURN NULL;
					END;
				$$;
				CREATE TRIGGER "viewsAfterInsert" AFTER INSERT ON "views"
					REFERENCING NEW TABLE AS NEW_ROWS
					FOR EACH STATEMENT EXECUTE FUNCTION "viewsAfterInsertFunc"();
				CREATE TRIGGER "viewsNotAllowed" BEFORE UPDATE OR DELETE OR TRUNCATE ON "views"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				-- "committedChangesets" table contains which changesets are explicitly committed to which views.
				CREATE TABLE "committedChangesets" (
					-- Changeset which belongs to the view. Also all changesets belonging to ancestors
					-- (as defined by view's path) of the view belong to the view, but we do not store
					-- them explicitly. The set of changesets belonging to the view should be kept
					-- consistent so that a new changeset is added to the view only if all ancestor
					-- changesets are already present in the view or in its ancestor views.
					"changeset" text STORAGE PLAIN COLLATE "C" NOT NULL,
					-- ID of the view.
					"view" text STORAGE PLAIN COLLATE "C" NOT NULL,
					-- Revision of this committed changeset.
					"revision" bigint NOT NULL,
					"metadata" `+s.MetadataType+` NOT NULL,
					PRIMARY KEY ("changeset", "view", "revision")
				);
				CREATE FUNCTION "committedChangesetsAfterInsertFunc"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						INSERT INTO "currentCommittedChangesets"
							SELECT DISTINCT ON ("changeset", "view") "changeset", "view", "revision" FROM NEW_ROWS
								ORDER BY "changeset", "view", "revision" DESC
								ON CONFLICT ("changeset", "view") DO UPDATE
									SET "revision"=EXCLUDED."revision";
						RETURN NULL;
					END;
				$$;
				CREATE TRIGGER "committedChangesetsAfterInsert" AFTER INSERT ON "committedChangesets"
					REFERENCING NEW TABLE AS NEW_ROWS
					FOR EACH STATEMENT EXECUTE FUNCTION "committedChangesetsAfterInsertFunc"();
				CREATE TRIGGER "committedChangesetsNotAllowed" BEFORE UPDATE OR DELETE OR TRUNCATE ON "committedChangesets"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				-- "currentViews" is automatically maintained table with the current (highest)
				-- revision of each view from table "views".
				CREATE TABLE "currentViews" (
					-- A subset of "views" columns.
					"view" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"revision" bigint NOT NULL,
					-- Having "name" here allows easy querying by name and also makes it easy for us to enforce
					-- the property we want: that each name is used by only one view at every given moment.
					"name" text UNIQUE,
					PRIMARY KEY ("view")
				);
				CREATE TRIGGER "currentViewsNotAllowed" BEFORE DELETE OR TRUNCATE ON "currentViews"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				-- "currentChanges" is automatically maintained table with the current (highest)
				-- revision of each change from table "changes".
				CREATE TABLE "currentChanges" (
					-- A subset of "changes" columns.
					"changeset" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"id" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"revision" bigint NOT NULL,
					PRIMARY KEY ("changeset", "id")
				);
				CREATE INDEX ON "currentChanges" USING btree ("id");
				CREATE TRIGGER "currentChangesNotAllowed" BEFORE TRUNCATE ON "currentChanges"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				-- "currentCommittedChangesets" is automatically maintained table with the current (highest)
				-- revision of each committed changeset from table "committedChangesets".
				CREATE TABLE "currentCommittedChangesets" (
					-- A subset of "committedChangesets" columns.
					"changeset" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"view" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"revision" bigint NOT NULL,
					PRIMARY KEY ("changeset", "view")
				);
				CREATE INDEX ON "currentCommittedChangesets" USING btree ("view");
				CREATE TRIGGER "currentCommittedChangesetsNotAllowed" BEFORE DELETE OR TRUNCATE ON "currentCommittedChangesets"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				-- "committedValues" is automatically maintained table of all changesets reachable from
				-- changesets explicitly committed to each view.
				CREATE TABLE "committedValues" (
					"view" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"id" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"changeset" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"depth" bigint NOT NULL,
					PRIMARY KEY ("view", "id", "changeset"),
					-- We allow only one version of the value per view at depth 0.
					-- We cannot use UNIQUE constraint because we want a WHERE predicate and we cannot use UNIQUE index
					-- because we want a deferred constraint which is checked after all changes to "committedValues" are
					-- done and not after every individual change (otherwise it can happen that the UNIQUE index raises
					-- an exception because duplicate values are encountered before everything is updated).
					CONSTRAINT "committedValuesLatest" EXCLUDE USING btree ("view" WITH =, "id" WITH =) WHERE ("depth"=0) DEFERRABLE INITIALLY DEFERRED
				);
				CREATE TRIGGER "committedValuesNotAllowed" BEFORE DELETE OR TRUNCATE ON "committedValues"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				CREATE FUNCTION "changesetCreate"(_changeset text, _id text, _parentChangesets text[], _value `+s.DataType+`, _metadata `+s.MetadataType+patchesArgument+`)
					RETURNS void LANGUAGE plpgsql AS $$
					BEGIN
						-- Changeset should not be committed (to any view).
						PERFORM 1 FROM "currentCommittedChangesets" WHERE "changeset"=_changeset LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset already committed' USING ERRCODE = '`+errorCodeAlreadyCommitted+`';
						END IF;
						IF _parentChangesets<>'{}' THEN
							-- Parent changesets should exist for ID. Query should work even if
							-- changesets are repeated in _parentChangesets.
							PERFORM 1 FROM "currentChanges" JOIN UNNEST(_parentChangesets) AS "changeset" USING ("changeset")
								WHERE "id"=_id
								HAVING COUNT(*)=array_length(_parentChangesets, 1);
							IF NOT FOUND THEN
								RAISE EXCEPTION 'invalid parent changeset' USING ERRCODE = '`+errorCodeParentInvalid+`';
							END IF;
						END IF;
						INSERT INTO "changes" VALUES (_changeset, _id, 1, _parentChangesets, '{}', _value, _metadata`+patchesValue+`);
					END;
				$$;

				CREATE FUNCTION "changesetDiscard"(_changeset text)
					RETURNS void LANGUAGE plpgsql AS $$
					BEGIN
						-- Changeset should not be committed (to any view).
						PERFORM 1 FROM "currentCommittedChangesets" WHERE "changeset"=_changeset LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset already committed' USING ERRCODE = '`+errorCodeAlreadyCommitted+`';
						END IF;
						-- Changeset should not be in use.
						PERFORM 1 FROM "currentChanges" JOIN "changes" USING ("changeset", "id", "revision")
							WHERE "parentChangesets"@>ARRAY[_changeset] LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset in use' USING ERRCODE = '`+errorCodeInUse+`';
						END IF;
						-- Discarding an empty (or an already discarded) changeset is not an error.
						DELETE FROM "changes" WHERE "changeset"=_changeset;
					END;
				$$;

				CREATE FUNCTION "changesetCommit"(_changeset text, _metadata `+s.MetadataType+`, _name text)
					RETURNS text[] LANGUAGE plpgsql AS $$
					DECLARE
						_view text;
						_path text[];
						_changesetsToCommit text[];
					BEGIN
						-- The view should exist.
						SELECT "view", "path" INTO _view, _path
							FROM "currentViews" JOIN "views" USING ("view", "revision")
							WHERE "currentViews"."name"=_name;
						IF NOT FOUND THEN
							RAISE EXCEPTION 'view not found' USING ERRCODE = '`+errorCodeViewNotFound+`';
						END IF;
						-- There must be at least one change in the changeset we want to commit. We know that parent
						-- changesets exist and do have (relevant) changes because we check that when creating changes.
						PERFORM 1 FROM "currentChanges" WHERE "changeset"=_changeset LIMIT 1;
						IF NOT FOUND THEN
							RAISE EXCEPTION 'changeset not found' USING ERRCODE = '`+errorCodeChangesetNotFound+`';
						END IF;
						-- Determine the list of changesets to commit: the changeset and any non-committed ancestor changesets.
						WITH RECURSIVE "viewChangesets" AS (
							SELECT "changeset" FROM "currentCommittedChangesets" WHERE "view"=ANY(_path)
						), "changesetsToCommit"("changeset") AS (
								VALUES (_changeset COLLATE "C")
							-- We use UNION and not UNION ALL here because we need distinct values in _changesetsToCommit
							-- because we have to insert only one row for each changeset into "committedChangesets".
							UNION
								-- We use LEFT JOIN with DISTINCT on the left table because it seems faster than EXCEPT,
								-- but we should validate and keep validating this. DISTINCT here is not critical, but
								-- its goal is to do less work in LEFT JOIN and later UNION.
								SELECT l."changeset"
									FROM (
										SELECT DISTINCT UNNEST("parentChangesets") AS "changeset"
											FROM "currentChanges"
												JOIN "changes" USING ("changeset", "id", "revision")
												JOIN "changesetsToCommit" USING ("changeset")
									) AS l
										LEFT JOIN "viewChangesets" AS r
										ON (l."changeset"=r."changeset")
									WHERE r."changeset" IS NULL
						)
						SELECT array_agg("changeset") INTO _changesetsToCommit FROM "changesetsToCommit";
						-- This raises unique violation if the provided changeset is already committed
						-- (we added ancestor changesets to _changesetsToCommit because they are not committed so
						-- we know they are not the ones raising unique violation, only the provided changeset can).
						INSERT INTO "committedChangesets" SELECT "changeset", _view, 1, _metadata FROM UNNEST(_changesetsToCommit) AS "changeset";
						-- Determine reachable changesets for all values in the changeset we want to commit.
						-- Other changesets from _changesetsToCommit should be found again as well.
						WITH RECURSIVE "reachableChangesets"("changeset", "id", "depth") AS (
								SELECT "changeset", "id", 0
									FROM "currentChanges" WHERE "changeset"=_changeset
							UNION ALL
								-- "parentChangesets" can contain duplicates.
								SELECT p.*, "id", "depth"+1
									FROM "currentChanges"
										JOIN "changes" USING ("changeset", "id", "revision")
										JOIN "reachableChangesets" USING ("changeset", "id"),
										-- We have to use LATERAL plus a sub-query to be able to use DISTINCT.
										LATERAL (SELECT DISTINCT UNNEST("parentChangesets")) AS p("changeset")
						)
						INSERT INTO "committedValues" SELECT DISTINCT ON ("changeset", "id") _view, "id", "changeset", "depth"
							FROM "reachableChangesets"
							-- We pick the smallest depth to be deterministic when there are multiple paths to the same changeset.
							-- If multiple paths lead to the same changeset at the same depth, we also insert a changeset
							-- only once (we have DISTINCT ON "changeset").
							ORDER BY "changeset", "id", "depth" ASC
							ON CONFLICT ("view", "id", "changeset") DO UPDATE
								SET "depth"=EXCLUDED."depth"
								-- No need to update if nothing has changed.
								WHERE "committedValues"."depth"<>EXCLUDED."depth";
					-- If we now have multiple rows with same ("view", "id", *, 0) combination we want an exception
					-- and we have for that an EXCLUDE constraint with such condition. We use a DEFERRABLE INITIALLY
					-- DEFERRED constraint so that INSERT above have time to modify multiple rows and only after it
					-- has done its work we force the constraint using SET CONSTRAINTS.
					-- This can happen if _changesetsToCommit are introducing multiple parallel coexisting versions
					-- of a value which we do not allow. We want that at any point, i.e., after a set of changesets
					-- are committed, there is only one version of a value per view. Branching can happen but everything
					-- has to be merged back together into one version before a set of changesets is committed.
					SET CONSTRAINTS "committedValuesLatest" IMMEDIATE;
					RETURN _changesetsToCommit;
					END;
				$$;
			`)
			if err != nil {
				return internal.WithPgxError(err)
			}

			viewID := identifier.New()
			_, err = tx.Exec(ctx, `INSERT INTO "views" VALUES ($1, 1, $2, $3, '{}')`, viewID.String(), MainView, []string{viewID.String()})
			if err != nil {
				return internal.WithPgxError(err)
			}

			err = tx.Commit(ctx)
			if err != nil {
				return internal.WithPgxError(err)
			}
		}

		return nil
	})
	if errE != nil {
		return errE
	}

	s.dbpool = dbpool

	return nil
}
