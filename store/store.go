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

type Store[Data, Metadata, Patch any] struct {
	Schema    string
	Committed chan<- Changeset[Data, Metadata, Patch]

	dbpool         *pgxpool.Pool
	patchesEnabled bool
}

func (s *Store[Data, Metadata, Patch]) tryCreateSchema(ctx context.Context, tx pgx.Tx) (bool, errors.E) {
	_, err := tx.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, s.Schema))
	if err != nil {
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) {
			// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
			switch pgError.Code {
			// unique_violation.
			case "23505":
				return false, nil
			// duplicate_schema.
			case "42P06":
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
					"patches" bytea[] NOT NULL,
				`
			}

			// TODO: Add a constraint that no two values with same ID should be created in multiple changesets.
			// TODO: Check if DESC should be specified for revision column.
			//       See: https://www.postgresql.org/message-id/CAKLmikNCFD44VjzRCRwuiVWDOE=T7zsOzygd5XakKNdRgLv-Aw@mail.gmail.com
			_, err := tx.Exec(ctx, `
				CREATE TABLE "changes" (
					-- ID of the value.
					"id" text NOT NULL,
					-- ID of the changeset this change belongs to.
					"changeset" text NOT NULL,
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
					"data" bytea,
					"metadata" bytea NOT NULL,
					`+patches+`
					PRIMARY KEY ("id", "changeset", "revision")
				)
			`)
			if err != nil {
				return internal.WithPgxError(err)
			}

			// TODO: Add constraints on name field.
			//       We want a) to allow a name to be associated only with one view ID at its highest revision
			//       b) view can start without name and name can be added later c) name can be removed from
			//       a view at a later time d) some other view can then get the name.
			// TODO: Check if DESC should be specified for revision column.
			//       See: https://www.postgresql.org/message-id/CAKLmikNCFD44VjzRCRwuiVWDOE=T7zsOzygd5XakKNdRgLv-Aw@mail.gmail.com
			_, err = tx.Exec(ctx, `
				CREATE TABLE "views" (
					-- ID of the view.
					"id" text NOT NULL,
					-- Revision of this view.
					"revision" bigint NOT NULL,
					-- Name of the view. Optional.
					"name" text,
					-- Path of view IDs starting with the current view, then the
					-- parent view, and then all further ancestors.
					"path" text[] NOT NULL,
					"metadata" bytea NOT NULL,
					PRIMARY KEY ("id", "revision")
				)
			`)
			if err != nil {
				return internal.WithPgxError(err)
			}
			_, err = tx.Exec(ctx, `
				CREATE INDEX ON "views" USING btree ("name")
			`)
			if err != nil {
				return internal.WithPgxError(err)
			}
			_, err = tx.Exec(ctx, `
				CREATE INDEX ON "views" USING gin ("path")
			`)
			if err != nil {
				return internal.WithPgxError(err)
			}

			_, err = tx.Exec(ctx, `
				CREATE TABLE "viewChangesets" (
					-- ID of the view.
					"id" text NOT NULL,
					-- Changeset which belongs to this view. Also all changesets belonging to ancestors
					-- (as defined by view's path) of this view belong to this view, but we do not store
					-- them explicitly. The set of changesets belonging to the view should be kept
					-- consistent so that every time a new changeset is added to the view, all ancestor
					-- changesets are added as well, unless they are already present in ancestor views.
					"changeset" text NOT NULL,
					"metadata" bytea NOT NULL,
					PRIMARY KEY ("id", "changeset")
				)
			`)
			if err != nil {
				return internal.WithPgxError(err)
			}

			viewID := identifier.New()
			_, err = tx.Exec(ctx, `INSERT INTO "views" VALUES ($1, 1, $2, $3, '{}')`, viewID.String(), MainView, []string{viewID.String()})
			if err != nil {
				return internal.WithPgxError(err)
			}

			_, err = tx.Exec(ctx, `
				CREATE VIEW "currentViews" AS
					SELECT DISTINCT ON ("id") * FROM "views" ORDER BY "id", "revision" DESC
			`)
			if err != nil {
				return internal.WithPgxError(err)
			}

			_, err = tx.Exec(ctx, `
				CREATE VIEW "currentChanges" AS
					SELECT DISTINCT ON ("id", "changeset") * FROM "changes" ORDER BY "id", "changeset", "revision" DESC
			`)
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
