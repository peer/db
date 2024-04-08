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
	errorCodeNotAllowed       = "P1000"
	errorCodeAlreadyCommitted = "P1001"
)

type Store[Data, Metadata, Patch any] struct {
	Schema       string
	Committed    chan<- Changeset[Data, Metadata, Patch]
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

func (s *Store[Data, Metadata, Patch]) Init(ctx context.Context, dbpool *pgxpool.Pool) (errE errors.E) { //nolint:nonamedreturns
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
			if s.patchesEnabled {
				patches = `
					-- Forward patches which bring parentChangesets versions of the value to
					-- this version of the value. If patches are available, the number of patches
					-- and their order must match that of parentChangesets. All patches have to
					-- end up with the equal value.
					"patches" ` + s.PatchType + `[] NOT NULL,
				`
			}

			// TODO: Add a constraint that no two values with same ID should be created in multiple changesets.
			_, err := tx.Exec(ctx, `
				CREATE FUNCTION "doNotAllow"() RETURNS TRIGGER LANGUAGE plpgsql AS $$
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
				CREATE FUNCTION "changesAfterInsertFunc"() RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						-- None of inserted changesets should be committed (to any view).
						PERFORM 1 FROM NEW_ROWS JOIN "currentCommittedChangesets" USING ("changeset") LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'already committed' USING ERRCODE = '`+errorCodeAlreadyCommitted+`';
						END IF;
						INSERT INTO "currentChanges"
							SELECT DISTINCT ON ("changeset", "id") "changeset", "id", "revision" FROM NEW_ROWS
								ORDER BY "changeset", "id", "revision" DESC
								ON CONFLICT ("changeset", "id") DO UPDATE
									SET "revision"=EXCLUDED."revision";
						RETURN NULL;
					END;
				$$;
				CREATE FUNCTION "changesAfterDeleteFunc"() RETURNS TRIGGER LANGUAGE plpgsql AS $$
					BEGIN
						-- None of deleted changesets should be committed (to any view).
						PERFORM 1 FROM OLD_ROWS JOIN "currentCommittedChangesets" USING ("changeset") LIMIT 1;
						IF FOUND THEN
							RAISE EXCEPTION 'already committed' USING ERRCODE = '`+errorCodeAlreadyCommitted+`';
						END IF;
						-- Currently, in Discard, we delete all revisions for all changes for a changeset
						-- at once. We take advantage of this here and just delete all changes for
						-- deleted changesets from "currentChanges" as well.
						DELETE FROM "currentChanges" USING OLD_ROWS
							WHERE "currentChanges"."changeset"=OLD_ROWS."changeset";
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
				CREATE FUNCTION "viewsAfterInsertFunc"() RETURNS TRIGGER LANGUAGE plpgsql AS $$
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
				CREATE FUNCTION "committedChangesetsAfterInsertFunc"() RETURNS TRIGGER LANGUAGE plpgsql AS $$
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
