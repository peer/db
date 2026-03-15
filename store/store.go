// Package store provides a versioned object store.
//
// This is a low-level component.
package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/internal/store"
)

// TODO: Implement "meld" functionality: allowing two values with different IDs to be merged into one.
//       From that moment on, both IDs should resolve to this new value (e.g., even with later changes,
//       for the latest version of the value returns the same value for both IDs).
//       One ID becomes the main one though and all changes are then tracked under that ID.

// TODO: Support "hard-forking" a value. In this case history is preserved, but a new identity is made for the value.

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

// CommittedChangesets represents all changesets committed together in one commit to a view.
type CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any] struct {
	// Seq is the sequential number assigned to this commit in the commit log.
	// It reflects the order in which commits were made to the database.
	Seq int64
	// Changesets are all changesets committed together in this commit.
	Changesets []Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
	View       View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
}

// WithStore returns a new CommittedChangesets object with
// all changesets and view associated with the given Store.
func (c CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) WithStore(
	ctx context.Context, store *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch],
) (CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	changesets := make([]Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], 0, len(c.Changesets))
	for _, cs := range c.Changesets {
		csWithStore, errE := cs.WithStore(ctx, store)
		if errE != nil {
			return CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{}, errE
		}
		changesets = append(changesets, csWithStore)
	}
	view, errE := c.View.WithStore(ctx, store)
	if errE != nil {
		return CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{}, errE
	}
	return CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
		Seq:        c.Seq,
		Changesets: changesets,
		View:       view,
	}, errE
}

// Store is a key-value store which preserves history of changes.
//
// For every change, a new value, its metadata, and optional forward
// patches are stored. Go types for them you configure with type parameters.
// You can use special None type to configure the Store instance to not
// use nor store patches.
type Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch any] struct {
	// Prefix to use when initializing PostgreSQL objects used by this store.
	Prefix string

	// PostgreSQL column types to store data, metadata, and patches.
	// It should probably be one of the jsonb, bytea, or text.
	// Go types used for Store type parameters should be compatible with
	// column types chosen.
	DataType     string
	MetadataType string
	PatchType    string

	// CommittedSize is the size of the channel to which one CommittedChangesets is sent for each commit.
	//
	// Set to a negative value to disable creating the channel.
	CommittedSize int `exhaustruct:"optional"`

	// A channel to which one CommittedChangesets is sent for each commit.
	// The changesets and view objects sent do not have an associated Store.
	//
	// CommittedChangesets are sent in the order in which commits were serialized
	// by the database, as reflected by each CommittedChangesets's Seq field.
	//
	// Channel is created by the listener when started and recreated on reconnection.
	Committed x.RecreatableChannel[CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]] `exhaustruct:"optional"`

	dbpool         *pgxpool.Pool
	patchesEnabled bool
	committed      chan<- CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
}

// Init initializes the Store.
//
// It creates and configures the PostgreSQL tables, indices, and
// stored procedures if they do not already exist.
//
// A non-nil listener is required when the Committed channel is set.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Init( //nolint:maintidx
	ctx context.Context, dbpool *pgxpool.Pool, listener *store.Listener,
) errors.E {
	if s.dbpool != nil {
		return errors.New("already initialized")
	}

	s.patchesEnabled = !isNoneType[Patch]()

	// TODO: Use schema management/migration instead.
	errE := store.RetryTransaction(ctx, dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
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

		//nolint:lll
		_, err := tx.Exec(ctx, `
				CREATE FUNCTION "`+s.Prefix+`DoNotAllow"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						RAISE EXCEPTION 'not allowed' USING ERRCODE='`+errorCodeNotAllowed+`';
					END;
				$$;

				-- "Changes" table contains all changes to values.
				CREATE TABLE "`+s.Prefix+`Changes" (
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
					-- value is being hard-forked from the existing one (in this case history is preserved,
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
				CREATE INDEX ON "`+s.Prefix+`Changes" USING gin ("parentChangesets");
				CREATE FUNCTION "`+s.Prefix+`ChangesAfterInsertFunc"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						INSERT INTO "`+s.Prefix+`CurrentChanges"
							SELECT DISTINCT ON ("changeset", "id") "changeset", "id", "revision" FROM NEW_ROWS
								ORDER BY "changeset", "id", "revision" DESC
								ON CONFLICT ("changeset", "id") DO UPDATE
									SET "revision"=EXCLUDED."revision";
						RETURN NULL;
					END;
				$$;
				CREATE FUNCTION "`+s.Prefix+`ChangesAfterDeleteFunc"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						DELETE FROM "`+s.Prefix+`CurrentChanges" USING OLD_ROWS
							WHERE "`+s.Prefix+`CurrentChanges"."changeset"=OLD_ROWS."changeset"
								AND "`+s.Prefix+`CurrentChanges"."id"=OLD_ROWS."id";
						INSERT INTO "`+s.Prefix+`CurrentChanges"
							SELECT DISTINCT ON ("changeset", "id") "changeset", "id", "`+s.Prefix+`Changes"."revision"
								FROM OLD_ROWS JOIN "`+s.Prefix+`Changes" USING ("changeset", "id")
								ORDER BY "changeset", "id", "`+s.Prefix+`Changes"."revision" DESC
								ON CONFLICT ("changeset", "id") DO UPDATE
									SET "revision"=EXCLUDED."revision";
						RETURN NULL;
					END;
				$$;
				CREATE TRIGGER "`+s.Prefix+`ChangesAfterInsert" AFTER INSERT ON "`+s.Prefix+`Changes"
					REFERENCING NEW TABLE AS NEW_ROWS
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`ChangesAfterInsertFunc"();
				CREATE TRIGGER "`+s.Prefix+`ChangesAfterDelete" AFTER DELETE ON "`+s.Prefix+`Changes"
					REFERENCING OLD TABLE AS OLD_ROWS
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`ChangesAfterDeleteFunc"();
				CREATE TRIGGER "`+s.Prefix+`ChangesNotAllowed" BEFORE UPDATE OR TRUNCATE ON "`+s.Prefix+`Changes"
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`DoNotAllow"();

				-- "Views" table contains all changes to views.
				CREATE TABLE "`+s.Prefix+`Views" (
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
				CREATE INDEX ON "`+s.Prefix+`Views" USING btree ("name");
				CREATE FUNCTION "`+s.Prefix+`ViewsAfterInsertFunc"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						INSERT INTO "`+s.Prefix+`CurrentViews"
							SELECT DISTINCT ON ("view") "view", "revision", "name" FROM NEW_ROWS
								ORDER BY "view", "revision" DESC
								ON CONFLICT ("view") DO UPDATE
									SET "revision"=EXCLUDED."revision", "name"=EXCLUDED."name";
						RETURN NULL;
					END;
				$$;
				CREATE TRIGGER "`+s.Prefix+`ViewsAfterInsert" AFTER INSERT ON "`+s.Prefix+`Views"
					REFERENCING NEW TABLE AS NEW_ROWS
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`ViewsAfterInsertFunc"();
				CREATE TRIGGER "`+s.Prefix+`ViewsNotAllowed" BEFORE UPDATE OR DELETE OR TRUNCATE ON "`+s.Prefix+`Views"
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`DoNotAllow"();

				-- "CommittedChangesets" table contains which changesets are explicitly committed to which views.
				CREATE TABLE "`+s.Prefix+`CommittedChangesets" (
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
				CREATE FUNCTION "`+s.Prefix+`CommittedChangesetsAfterInsertFunc"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						INSERT INTO "`+s.Prefix+`CurrentCommittedChangesets"
							SELECT DISTINCT ON ("changeset", "view") "changeset", "view", "revision" FROM NEW_ROWS
								ORDER BY "changeset", "view", "revision" DESC
								ON CONFLICT ("changeset", "view") DO UPDATE
									SET "revision"=EXCLUDED."revision";
						RETURN NULL;
					END;
				$$;
				CREATE TRIGGER "`+s.Prefix+`CommittedChangesetsAfterInsert" AFTER INSERT ON "`+s.Prefix+`CommittedChangesets"
					REFERENCING NEW TABLE AS NEW_ROWS
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`CommittedChangesetsAfterInsertFunc"();
				CREATE TRIGGER "`+s.Prefix+`CommittedChangesetsNotAllowed" BEFORE UPDATE OR DELETE OR TRUNCATE ON "`+s.Prefix+`CommittedChangesets"
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`DoNotAllow"();

				-- "CurrentViews" is automatically maintained table with the current (highest)
				-- revision of each view from table "Views".
				CREATE TABLE "`+s.Prefix+`CurrentViews" (
					-- A subset of "Views" columns.
					"view" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"revision" bigint NOT NULL,
					-- Having "name" here allows easy querying by name and also makes it easy for us to enforce
					-- the property we want: that each name is used by only one view at every given moment.
					"name" text UNIQUE,
					PRIMARY KEY ("view")
				);
				CREATE TRIGGER "`+s.Prefix+`CurrentViewsNotAllowed" BEFORE DELETE OR TRUNCATE ON "`+s.Prefix+`CurrentViews"
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`DoNotAllow"();

				-- "`+s.Prefix+`CurrentChanges" is automatically maintained table with the current (highest)
				-- revision of each change from table "Changes".
				CREATE TABLE "`+s.Prefix+`CurrentChanges" (
					-- A subset of "Changes" columns.
					"changeset" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"id" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"revision" bigint NOT NULL,
					PRIMARY KEY ("changeset", "id")
				);
				CREATE INDEX ON "`+s.Prefix+`CurrentChanges" USING btree ("id");
				CREATE TRIGGER "`+s.Prefix+`CurrentChangesNotAllowed" BEFORE TRUNCATE ON "`+s.Prefix+`CurrentChanges"
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`DoNotAllow"();

				-- "CurrentCommittedChangesets" is automatically maintained table with the current (highest)
				-- revision of each committed changeset from table "CommittedChangesets".
				CREATE TABLE "`+s.Prefix+`CurrentCommittedChangesets" (
					-- A subset of "CommittedChangesets" columns.
					"changeset" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"view" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"revision" bigint NOT NULL,
					PRIMARY KEY ("changeset", "view")
				);
				CREATE INDEX ON "`+s.Prefix+`CurrentCommittedChangesets" USING btree ("view");
				CREATE TRIGGER "`+s.Prefix+`CurrentCommittedChangesetsNotAllowed" BEFORE DELETE OR TRUNCATE ON "`+s.Prefix+`CurrentCommittedChangesets"
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`DoNotAllow"();

				-- "CommittedValues" is automatically maintained table of all changesets reachable from
				-- changesets explicitly committed to each view.
				CREATE TABLE "`+s.Prefix+`CommittedValues" (
					"view" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"id" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"changeset" text STORAGE PLAIN COLLATE "C" NOT NULL,
					"depth" bigint NOT NULL,
					PRIMARY KEY ("view", "id", "changeset"),
					-- We allow only one version of the value per view at depth 0.
					-- We cannot use UNIQUE constraint because we want a WHERE predicate and we cannot use UNIQUE index
					-- because we want a deferred constraint which is checked after all changes to "CommittedValues" are
					-- done and not after every individual change (otherwise it can happen that the UNIQUE index raises
					-- an exception because duplicate values are encountered before everything is updated).
					CONSTRAINT "`+s.Prefix+`CommittedValuesLatest" EXCLUDE USING btree ("view" WITH =, "id" WITH =) WHERE ("depth"=0) DEFERRABLE INITIALLY DEFERRED
				);
				CREATE TRIGGER "`+s.Prefix+`CommittedValuesNotAllowed" BEFORE DELETE OR TRUNCATE ON "`+s.Prefix+`CommittedValues"
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`DoNotAllow"();

				CREATE FUNCTION "`+s.Prefix+`ChangesetCreate"(_changeset text, _id text, _parentChangesets text[], _value `+s.DataType+`, _metadata `+s.MetadataType+patchesArgument+`)
					RETURNS void LANGUAGE plpgsql AS $$
					BEGIN
						-- Changeset should not be committed (to any view).
						PERFORM 1 FROM "`+s.Prefix+`CurrentCommittedChangesets" WHERE "changeset"=_changeset LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset already committed' USING ERRCODE='`+errorCodeAlreadyCommitted+`';
						END IF;
						IF _parentChangesets<>'{}' THEN
							-- Parent changesets should exist for ID. Query should work even if
							-- changesets are repeated in _parentChangesets.
							PERFORM 1 FROM "`+s.Prefix+`CurrentChanges" JOIN UNNEST(_parentChangesets) AS "changeset" USING ("changeset")
								WHERE "id"=_id
								HAVING COUNT(*)=array_length(_parentChangesets, 1);
							IF NOT FOUND THEN
								RAISE EXCEPTION 'invalid parent changeset' USING ERRCODE='`+errorCodeParentInvalid+`';
							END IF;
						END IF;
						INSERT INTO "`+s.Prefix+`Changes" VALUES (_changeset, _id, 1, _parentChangesets, '{}', _value, _metadata`+patchesValue+`);
					END;
				$$;

				CREATE FUNCTION "`+s.Prefix+`ChangesetDiscard"(_changeset text)
					RETURNS void LANGUAGE plpgsql AS $$
					BEGIN
						-- Changeset should not be committed (to any view).
						PERFORM 1 FROM "`+s.Prefix+`CurrentCommittedChangesets" WHERE "changeset"=_changeset LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset already committed' USING ERRCODE='`+errorCodeAlreadyCommitted+`';
						END IF;
						-- Changeset should not be in use.
						PERFORM 1 FROM "`+s.Prefix+`CurrentChanges" JOIN "`+s.Prefix+`Changes" USING ("changeset", "id", "revision")
							WHERE "parentChangesets"@>ARRAY[_changeset] LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset in use' USING ERRCODE='`+errorCodeInUse+`';
						END IF;
						-- Discarding an empty (or an already discarded) changeset is not an error.
						DELETE FROM "`+s.Prefix+`Changes" WHERE "changeset"=_changeset;
					END;
				$$;

				CREATE FUNCTION "`+s.Prefix+`ChangesetCommit"(_changeset text, _metadata `+s.MetadataType+`, _name text)
					RETURNS text[] LANGUAGE plpgsql AS $$
					DECLARE
						_view text;
						_path text[];
						_changesetsToCommit text[];
					BEGIN
						-- The view should exist.
						SELECT "view", "path" INTO _view, _path
							FROM "`+s.Prefix+`CurrentViews" JOIN "`+s.Prefix+`Views" USING ("view", "revision")
							WHERE "`+s.Prefix+`CurrentViews"."name"=_name;
						IF NOT FOUND THEN
							RAISE EXCEPTION 'view not found' USING ERRCODE='`+errorCodeViewNotFound+`';
						END IF;
						-- There must be at least one change in the changeset we want to commit. We know that parent
						-- changesets exist and do have (relevant) changes because we check that when creating changes.
						PERFORM 1 FROM "`+s.Prefix+`CurrentChanges" WHERE "changeset"=_changeset LIMIT 1;
						IF NOT FOUND THEN
							RAISE EXCEPTION 'changeset not found' USING ERRCODE='`+errorCodeChangesetNotFound+`';
						END IF;
						-- Determine the list of changesets to commit: the changeset and any non-committed ancestor changesets.
						WITH RECURSIVE "viewChangesets" AS (
							SELECT "changeset" FROM "`+s.Prefix+`CurrentCommittedChangesets" WHERE "view"=ANY(_path)
						), "changesetsToCommit"("changeset") AS (
								VALUES (_changeset COLLATE "C")
							-- We use UNION and not UNION ALL here because we need distinct values in _changesetsToCommit
							-- because we have to insert only one row for each changeset into "CommittedChangesets".
							UNION
								-- We use LEFT JOIN with DISTINCT on the left table because it seems faster than EXCEPT,
								-- but we should validate and keep validating this. DISTINCT here is not critical, but
								-- its goal is to do less work in LEFT JOIN and later UNION.
								SELECT l."changeset"
									FROM (
										SELECT DISTINCT UNNEST("parentChangesets") AS "changeset"
											FROM "`+s.Prefix+`CurrentChanges"
												JOIN "`+s.Prefix+`Changes" USING ("changeset", "id", "revision")
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
						INSERT INTO "`+s.Prefix+`CommittedChangesets" SELECT "changeset", _view, 1, _metadata FROM UNNEST(_changesetsToCommit) AS "changeset";
						-- Determine reachable changesets for all values in the changeset we want to commit.
						-- Other changesets from _changesetsToCommit should be found again as well.
						WITH RECURSIVE "reachableChangesets"("changeset", "id", "depth") AS (
								SELECT "changeset", "id", 0
									FROM "`+s.Prefix+`CurrentChanges" WHERE "changeset"=_changeset
							UNION ALL
								-- "parentChangesets" can contain duplicates.
								SELECT p.*, "id", "depth"+1
									FROM "`+s.Prefix+`CurrentChanges"
										JOIN "`+s.Prefix+`Changes" USING ("changeset", "id", "revision")
										JOIN "reachableChangesets" USING ("changeset", "id"),
										-- We have to use LATERAL plus a sub-query to be able to use DISTINCT.
										LATERAL (SELECT DISTINCT UNNEST("parentChangesets")) AS p("changeset")
						)
						INSERT INTO "`+s.Prefix+`CommittedValues" SELECT DISTINCT ON ("changeset", "id") _view, "id", "changeset", "depth"
							FROM "reachableChangesets"
							-- We pick the smallest depth to be deterministic when there are multiple paths to the same changeset.
							-- If multiple paths lead to the same changeset at the same depth, we also insert a changeset
							-- only once (we have DISTINCT ON "changeset").
							ORDER BY "changeset", "id", "depth" ASC
							ON CONFLICT ("view", "id", "changeset") DO UPDATE
								SET "depth"=EXCLUDED."depth"
								-- No need to update if nothing has changed.
								WHERE "`+s.Prefix+`CommittedValues"."depth"<>EXCLUDED."depth";
					-- If we now have multiple rows with same ("view", "id", *, 0) combination we want an exception
					-- and we have for that an EXCLUDE constraint with such condition. We use a DEFERRABLE INITIALLY
					-- DEFERRED constraint so that INSERT above have time to modify multiple rows and only after it
					-- has done its work we force the constraint using SET CONSTRAINTS.
					-- This can happen if _changesetsToCommit are introducing multiple parallel coexisting versions
					-- of a value which we do not allow. We want that at any point, i.e., after a set of changesets
					-- are committed, there is only one version of a value per view. Branching can happen but everything
					-- has to be merged back together into one version before a set of changesets is committed.
					SET CONSTRAINTS "`+s.Prefix+`CommittedValuesLatest" IMMEDIATE;
					INSERT INTO "`+s.Prefix+`CommitLog" ("view", "name", "changesets") VALUES (_view, _name, _changesetsToCommit);
					RETURN _changesetsToCommit;
					END;
				$$;

				-- "CommitLog" table serializes all commits with a sequential number.
				CREATE TABLE "`+s.Prefix+`CommitLog" (
					-- Sequential number of this commit, assigned at commit time.
					-- It reflects the order in which commits were serialized by the database.
					"seq" bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
					-- ID of the view the changesets were committed to.
					"view" text STORAGE PLAIN COLLATE "C" NOT NULL,
					-- Name of the view the changesets were committed to at commit time.
					-- View's name might change since the commit time and this is not represented here.
					"name" text NOT NULL,
					-- IDs of all changesets committed together in this commit.
					"changesets" text[] COLLATE "C" NOT NULL
				);
				CREATE FUNCTION "`+s.Prefix+`CommitLogAfterInsertFunc"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					DECLARE
						_payload text;
					BEGIN
						-- Build full JSON payload: {"seq":N,"name":"...","changesets":["cs1",...]}.
						-- If it exceeds 7900 bytes, fall back to {"seq":N} so the receiver
						-- fetches the remaining fields from the CommitLog table.
						_payload := json_build_object('seq', NEW."seq", 'name', NEW."name", 'changesets', NEW."changesets")::text;
						IF length(_payload) > 7900 THEN
							_payload := json_build_object('seq', NEW."seq")::text;
						END IF;
						PERFORM pg_notify('`+s.Prefix+`CommittedChangesets', _payload);
						RETURN NULL;
					END;
				$$;
				CREATE TRIGGER "`+s.Prefix+`CommitLogAfterInsert" AFTER INSERT ON "`+s.Prefix+`CommitLog"
					FOR EACH ROW EXECUTE FUNCTION "`+s.Prefix+`CommitLogAfterInsertFunc"();
				CREATE TRIGGER "`+s.Prefix+`CommitLogNotAllowed" BEFORE UPDATE OR DELETE OR TRUNCATE ON "`+s.Prefix+`CommitLog"
					FOR EACH STATEMENT EXECUTE FUNCTION "`+s.Prefix+`DoNotAllow"();
			`)
		if err != nil {
			return store.WithPgxError(err)
		}

		viewID := identifier.New()
		_, err = tx.Exec(ctx, `INSERT INTO "`+s.Prefix+`Views" VALUES ($1, 1, $2, $3, '{}')`, viewID.String(), MainView, []string{viewID.String()})
		if err != nil {
			return store.WithPgxError(err)
		}

		return nil
	})
	if errE != nil {
		if pgError, ok := errors.AsType[*pgconn.PgError](errE); ok {
			switch pgError.Code {
			case store.ErrorCodeUniqueViolation:
				// Nothing.
			case store.ErrorCodeDuplicateFunction:
				// Nothing.
			case store.ErrorCodeDuplicateTable:
				// Nothing.
			default:
				return errE
			}
		} else {
			return errE
		}
	}

	s.dbpool = dbpool

	if listener != nil {
		if s.CommittedSize >= 0 {
			listener.Handle(s.Prefix+"CommittedChangesets", s)
		}
	}

	return nil
}

// HandleNotification implements pgxlisten.Handler interface.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) HandleNotification(
	ctx context.Context, notification *pgconn.Notification, conn *pgx.Conn,
) error {
	switch notification.Channel {
	case s.Prefix + "CommittedChangesets":
		return s.handleCommittedChangesets(ctx, notification, conn)
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = notification.Channel
		return errE
	}
}

// HandleBacklog implements pgxlisten.BacklogHandler interface.
//
// It recreates channels to signal to their consumers that notifications might have been
// missed and that they should take corrective actions, if possible.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) HandleBacklog(
	_ context.Context, channel string, _ *pgx.Conn,
) error {
	switch channel {
	case s.Prefix + "CommittedChangesets":
		// CommittedSize should be >= 0 here unless it was changed after initialization which is not allowed.
		s.committed = s.Committed.Recreate(s.CommittedSize)
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = channel
		return errE
	}
	return nil
}

// HandlingReady implements store.Handler interface.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) HandlingReady(ctx context.Context, channel string) errors.E {
	switch channel {
	case s.Prefix + "CommittedChangesets":
		// We just wait for channel to be available. This means that HandleBacklog has completed.
		_, errE := s.Committed.Get(ctx)
		return errE
	default:
		errE := errors.New("unknown notification channel")
		errors.Details(errE)["channel"] = channel
		return errE
	}
}

// handleCommitLogNotification handles CommittedChangesets notifications and forwards
// the committed changesets to the Committed channel in commit order.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) handleCommittedChangesets(
	ctx context.Context, notification *pgconn.Notification, _ *pgx.Conn,
) error {
	// Payload is a JSON object. Full form: {"seq":N,"name":"...","changesets":["cs1",...]}.
	// When the full payload exceeds 7900 bytes, only {"seq":N} is sent
	// and changesets are fetched from the CommitLog table.
	var payload struct {
		Seq        int64    `json:"seq"`
		Name       string   `json:"name"`
		Changesets []string `json:"changesets"`
	}
	errE := x.UnmarshalWithoutUnknownFields([]byte(notification.Payload), &payload)
	if errE != nil {
		return errE
	}

	var changesets []string
	var viewName string
	if payload.Changesets != nil {
		// Full payload: no database fetch needed.
		viewName = payload.Name
		changesets = payload.Changesets
	} else {
		// Payload exceeded 7900 bytes: fetch changesets and view name from CommitLog.
		err := s.dbpool.QueryRow(ctx, `SELECT "changesets", "name" FROM "`+s.Prefix+`CommitLog" WHERE "seq"=$1`, payload.Seq).Scan(&changesets, &viewName)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// We send changesets and a view without store, requiring the receiver to use WithStore on them.
	commit := CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
		Seq:        payload.Seq,
		Changesets: make([]Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], 0, len(changesets)),
		View: View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
			name:  viewName,
			store: nil,
		},
	}
	// There might be more than just one changeset committed if its parent changesets were not committed before.
	for _, changesetID := range changesets {
		commit.Changesets = append(commit.Changesets, Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
			id:    identifier.String(changesetID),
			store: nil,
		})
	}
	select {
	case s.committed <- commit:
	case <-ctx.Done():
	}
	return nil
}
