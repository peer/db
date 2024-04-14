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

type None *struct{}

const MainView = "main"

const (
	// Our error codes.
	errorCodeNotAllowed        = "P1000"
	errorCodeAlreadyCommitted  = "P1001"
	errorCodeInUse             = "P1002"
	errorCodeParentInvalid     = "P1003"
	errorCodeViewNotFound      = "P1004"
	errorCodeChangesetNotFound = "P1005"
)

type Store[Data, Metadata, Patch any] struct {
	Schema string

	// The order in which changesets are send to the channel is not necessary
	// the order in which they were committed. You should not relay on the order.
	Committed chan<- Changeset[Data, Metadata, Patch]

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
			patchesEmptyValue := ""
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
				patchesEmptyValue = ", '{}'"
				patchesArgument = ", _patches " + s.PatchType + "[]"
				patchesValue = ", _patches"
			}

			//nolint:goconst
			_, err := tx.Exec(ctx, `
				CREATE FUNCTION "doNotAllow"()
					RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						RAISE EXCEPTION 'not allowed' USING ERRCODE = '`+errorCodeNotAllowed+`';
					END;
				$$;

				CREATE TABLE "changes" (
					-- ID of the changeset this change belongs to.
					"changeset" text NOT NULL,
					-- ID of the value.
					"id" text NOT NULL,
					-- Revision of this change.
					"revision" bigint NOT NULL,
					-- IDs of changesets this value has been changed the last before this change.
					-- The same changeset ID can happen to repeat when melding multiple values
					-- (parentIds is then set, too).
					"parentChangesets" text[] NOT NULL,
					-- Direct previous IDs of this value. Multiple if this change is melding multiple
					-- values (number of IDs and order matches parentChangesets). Only one ID if a new
					-- value is being forked from the existing one (in this case history is preserved,
					-- but a new identity is made). An empty array means that the ID has not changed.
					-- The same parent ID can happen to repeat when both merging and melding at the
					-- same time.
					"parentIds" text[] NOT NULL DEFAULT '{}',
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

				CREATE TABLE "views" (
					-- ID of the view.
					"view" text NOT NULL,
					-- Revision of this view.
					"revision" bigint NOT NULL,
					-- Name of the view. Optional.
					"name" text,
					-- Path of view IDs starting with the current view, then the
					-- parent view, and then all further ancestors.
					"path" text[] NOT NULL,
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

				CREATE TABLE "committedChangesets" (
					-- Changeset which belongs to the view. Also all changesets belonging to ancestors
					-- (as defined by view's path) of the view belong to the view, but we do not store
					-- them explicitly. The set of changesets belonging to the view should be kept
					-- consistent so that a new changeset is added to the view only if all ancestor
					-- changesets are already present in the view or in its ancestor views.
					"changeset" text NOT NULL,
					-- ID of the view.
					"view" text NOT NULL,
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

				CREATE TABLE "currentViews" (
					-- A subset of "views" columns.
					"view" text NOT NULL,
					"revision" bigint NOT NULL,
					-- Having "name" here allows easy querying by name and also makes it easy for us to enforce
					-- the property we want: that each name is used by only one view at every given moment.
					"name" text UNIQUE,
					PRIMARY KEY ("view")
				);
				CREATE TRIGGER "currentViewsNotAllowed" BEFORE DELETE OR TRUNCATE ON "currentViews"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				CREATE TABLE "currentChanges" (
					-- A subset of "changes" columns.
					"changeset" text NOT NULL,
					"id" text NOT NULL,
					"revision" bigint NOT NULL,
					PRIMARY KEY ("changeset", "id")
				);
				CREATE INDEX ON "currentChanges" USING btree ("id");
				CREATE TRIGGER "currentChangesNotAllowed" BEFORE TRUNCATE ON "currentChanges"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				CREATE TABLE "currentCommittedChangesets" (
					-- A subset of "committedChangesets" columns.
					"changeset" text NOT NULL,
					"view" text NOT NULL,
					"revision" bigint NOT NULL,
					PRIMARY KEY ("changeset", "view")
				);
				CREATE INDEX ON "currentCommittedChangesets" USING btree ("view");
				CREATE TRIGGER "currentCommittedChangesetsNotAllowed" BEFORE DELETE OR TRUNCATE ON "currentCommittedChangesets"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				CREATE TABLE "committedValues" (
					"view" text NOT NULL,
					"id" text NOT NULL,
					"changeset" text NOT NULL,
					PRIMARY KEY ("view", "id")
				);
				CREATE INDEX ON "committedValues" USING btree ("changeset");
				CREATE TRIGGER "committedValuesNotAllowed" BEFORE DELETE OR TRUNCATE ON "committedValues"
					FOR EACH STATEMENT EXECUTE FUNCTION "doNotAllow"();

				CREATE FUNCTION "changesetInsert"(_changeset text, _id text, _value `+s.DataType+`, _metadata `+s.MetadataType+`)
					RETURNS void LANGUAGE plpgsql AS $$
					BEGIN
						-- Changeset should not be committed (to any view).
						PERFORM 1 FROM "currentCommittedChangesets" WHERE "changeset"=_changeset LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset already committed' USING ERRCODE = '`+errorCodeAlreadyCommitted+`';
						END IF;
						INSERT INTO "changes" VALUES (_changeset, _id, 1, '{}', '{}', _value, _metadata`+patchesEmptyValue+`);
					END;
				$$;

				CREATE FUNCTION "changesetUpdate"(_changeset text, _id text, _parentChangesets text[], _value `+s.DataType+`, _metadata `+s.MetadataType+patchesArgument+`)
					RETURNS void LANGUAGE plpgsql AS $$
					BEGIN
						-- Changeset should not be committed (to any view).
						PERFORM 1 FROM "currentCommittedChangesets" WHERE "changeset"=_changeset LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset already committed' USING ERRCODE = '`+errorCodeAlreadyCommitted+`';
						END IF;
						-- Parent changesets should exist for ID. Query should work even if
						-- changesets are repeated in _parentChangesets.
						PERFORM 1 FROM "currentChanges" JOIN UNNEST(_parentChangesets) AS "changeset" USING ("changeset")
							WHERE "id"=_id
							HAVING COUNT(*)=array_length(_parentChangesets, 1);
						IF NOT FOUND THEN
							RAISE EXCEPTION 'invalid parent changeset' USING ERRCODE = '`+errorCodeParentInvalid+`';
						END IF;
						INSERT INTO "changes" VALUES (_changeset, _id, 1, _parentChangesets, '{}', _value, _metadata`+patchesValue+`);
					END;
				$$;

				CREATE FUNCTION "changesetReplace"(_changeset text, _id text, _parentChangesets text[], _value `+s.DataType+`, _metadata `+s.MetadataType+`)
					RETURNS void LANGUAGE plpgsql AS $$
					BEGIN
						-- Changeset should not be committed (to any view).
						PERFORM 1 FROM "currentCommittedChangesets" WHERE "changeset"=_changeset LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset already committed' USING ERRCODE = '`+errorCodeAlreadyCommitted+`';
						END IF;
						-- Parent changesets should exist for ID. Query should work even if
						-- changesets are repeated in _parentChangesets.
						PERFORM 1 FROM "currentChanges" JOIN UNNEST(_parentChangesets) AS "changeset" USING ("changeset")
							WHERE "id"=_id
							HAVING COUNT(*)=array_length(_parentChangesets, 1);
						IF NOT FOUND THEN
							RAISE EXCEPTION 'invalid parent changeset' USING ERRCODE = '`+errorCodeParentInvalid+`';
						END IF;
						INSERT INTO "changes" VALUES (_changeset, _id, 1, _parentChangesets, '{}', _value, _metadata`+patchesEmptyValue+`);
					END;
				$$;

				CREATE FUNCTION "changesetDelete"(_changeset text, _id text, _parentChangesets text[], _metadata `+s.MetadataType+`)
					RETURNS void LANGUAGE plpgsql AS $$
					BEGIN
						-- Changeset should not be committed (to any view).
						PERFORM 1 FROM "currentCommittedChangesets" WHERE "changeset"=_changeset LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'changeset already committed' USING ERRCODE = '`+errorCodeAlreadyCommitted+`';
						END IF;
						-- Parent changesets should exist for ID. Query should work even if
						-- changesets are repeated in _parentChangesets.
						PERFORM 1 FROM "currentChanges" JOIN UNNEST(_parentChangesets) AS "changeset" USING ("changeset")
							WHERE "id"=_id
							HAVING COUNT(*)=array_length(_parentChangesets, 1);
						IF NOT FOUND THEN
							RAISE EXCEPTION 'invalid parent changeset' USING ERRCODE = '`+errorCodeParentInvalid+`';
						END IF;
						INSERT INTO "changes" VALUES (_changeset, _id, 1, _parentChangesets, '{}', NULL, _metadata`+patchesEmptyValue+`);
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
								VALUES (_changeset)
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
						-- (ancestor changesets we added to _changesetsToCommit because they are not).
						INSERT INTO "committedChangesets" SELECT "changeset", _view, 1, _metadata FROM UNNEST(_changesetsToCommit) AS "changeset";
						WITH "values" AS (
							SELECT DISTINCT "id" FROM "currentChanges" JOIN UNNEST(_changesetsToCommit) AS "changeset" USING ("changeset")
						)
						-- For every ID in _changesetsToCommit we find the latest changeset.
						INSERT INTO "committedValues" SELECT _view, "id", (
								-- The latest changeset is among now updated "committedChangesets"/"currentCommittedChangesets" for the
								-- view and the value. It is not enough to look just in _changesetsToCommit changesets because there
								-- might be diverging changes introducing multiple latest changesets using earlier changesets
								-- (e.g., a changeset from _changesetsToCommit uses a parent changeset which is not the latest changeset).
								SELECT "changeset"
									FROM "currentCommittedChangesets"
										JOIN "currentChanges" USING ("changeset")
									WHERE "view"=ANY(_path) AND "id"="values"."id"
								EXCEPT
								-- But it is not a parent changeset of another changeset for the value.
								SELECT UNNEST("parentChangesets") AS "changeset"
									FROM "currentCommittedChangesets"
										JOIN ("currentChanges" JOIN "changes" USING ("changeset", "id", "revision")) USING ("changeset")
										WHERE "view"=ANY(_path) AND "id"="values"."id"
							) as "changeset" FROM "values"
							-- Here we rely on the fact that if there are multiple rows to be inserted with same ("view", "id")
							-- combination, this still fails with exception. This can happen if _changesetsToCommit are introducing
							-- multiple parallel coexisting versions of a value which we do not allow. We want that at any point, i.e.,
							-- after a set of changesets are committed, there is only one version of a value per view. Branches can
							-- happen but they have to be merged back together into one version before a set of changesets is committed.
							ON CONFLICT ("view", "id") DO UPDATE
								SET "changeset"=EXCLUDED."changeset";
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
