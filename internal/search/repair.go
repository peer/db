package search

import (
	"context"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/store"

	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

// materializedStateTables are the tables holding the bridge-maintained materialized state: rows derived
// from the stored documents under the current schema and site configuration, maintained incrementally as
// commits are indexed, and rebuildable from scratch by RebuildMaterializedState.
var materializedStateTables = []string{"References", "InverseRelations", "Embedding"} //nolint:gochecknoglobals

// MaterializedStateDrift reports, per materialized state table, how the table's current contents differ
// from a rebuild from the stored documents: Extra counts the rows present which the rebuild does not
// produce, and Missing counts the rows the rebuild produces which are not present.
type MaterializedStateDrift struct {
	ReferencesExtra         int64
	ReferencesMissing       int64
	InverseRelationsExtra   int64
	InverseRelationsMissing int64
	EmbeddingExtra          int64
	EmbeddingMissing        int64
}

// Any returns true when any table's contents differ from the rebuild.
func (d MaterializedStateDrift) Any() bool {
	return d.ReferencesExtra > 0 || d.ReferencesMissing > 0 ||
		d.InverseRelationsExtra > 0 || d.InverseRelationsMissing > 0 ||
		d.EmbeddingExtra > 0 || d.EmbeddingMissing > 0
}

// maxStagedRows is how many materialized state rows are accumulated before they are inserted into the
// staging tables. The batch is measured in rows and not in documents because a document contributes a row
// for every reference claim, visibility level, and value-hierarchy ancestor it reaches: batching a whole
// page of documents (store.MaxPageLength of them) would put millions of rows, and the parameter arrays
// carrying them, into a single statement.
const maxStagedRows = 50000

// stagedRows collects materialized state rows for batched insertion into the staging tables, as parallel
// argument arrays for the unnest-based inserts.
type stagedRows struct {
	RefTargets, RefLevels, RefClaims, RefSources                                 []string
	RefIsSources                                                                 []bool
	InvTargets, InvLevels, InvClaims, InvSources, InvTargetProps, InvSourceProps []string
	InvConfidences                                                               []float64
	EmbTargets, EmbEmbedders, EmbPaths                                           []string
}

// Len returns the number of staged rows across all three tables, which is what the batch is measured in.
func (r *stagedRows) Len() int {
	return len(r.RefTargets) + len(r.InvTargets) + len(r.EmbTargets)
}

// RebuildMaterializedState derives the references, inverse-relations, and embedding tables anew, from the
// latest versions of the stored documents as the single source of truth, interpreted under the current
// schema and site configuration (the converters and hooks set through Prepare), and replaces the tables'
// contents with the result when they differ. With dryRun nothing is replaced, only the drift is reported.
// The drift is reported in both modes, so a dry run doubles as a drift detector for the incremental
// maintenance the bridge does while indexing commits.
//
// It must run while nothing else writes the tables or renders documents from them: in a process whose
// bridge is not started, with no reindex job running, and never concurrently with a live process indexing
// the same store.
//
// When count and size are non-nil they track progress: size is increased once by the document total and
// count is increased as documents are processed, ending exactly at that total.
func (b *Bridge) RebuildMaterializedState(ctx context.Context, dryRun bool, count, size *x.Counter) (MaterializedStateDrift, errors.E) {
	drift := MaterializedStateDrift{}

	// The expected rows are streamed into staging tables (same schema as the real ones), so that the
	// comparison with the current contents and the replacement both happen inside PostgreSQL, without
	// holding the row set in memory.
	errE := b.recreateRepairStagingTables(ctx)
	if errE != nil {
		return drift, errE
	}

	total, errE := b.Store.Count(ctx, true)
	if errE != nil {
		return drift, errE
	}
	if size != nil {
		size.Add(total)
	}

	var processed int64
	var after *identifier.Identifier
	rows := &stagedRows{}
	for {
		// List returns every value id committed to the main view, including deleted ones, in id order, for
		// keyset pagination. The main view is the only view PeerDB commits to, and indexing reads documents
		// the same way, so the rebuild covers the same documents the incremental maintenance does.
		ids, errE := b.Store.List(ctx, nil, after)
		if errE != nil {
			return drift, errE
		}
		if len(ids) == 0 {
			break
		}

		for _, id := range ids {
			errE = b.stageDocument(ctx, id, rows)
			if errE != nil {
				return drift, errE
			}
			// The staged rows are flushed on their own count, so an insert carries the threshold plus
			// whatever the document which crossed it contributes, rather than everything a whole page
			// of documents contributes. A document's own rows are never split across inserts.
			if rows.Len() >= maxStagedRows {
				errE = b.insertStagedRows(ctx, rows)
				if errE != nil {
					return drift, errE
				}
				rows = &stagedRows{}
			}
		}

		processed += int64(len(ids))
		if count != nil {
			count.Add(int64(len(ids)))
		}
		if len(ids) < store.MaxPageLength {
			break
		}
		after = &ids[len(ids)-1]
	}

	// Flush what the last batch left below the threshold.
	errE = b.insertStagedRows(ctx, rows)
	if errE != nil {
		return drift, errE
	}

	// Reconcile so count was increased by exactly the total reported by Store.Count (matching the size
	// increase), even if the number of listed ids differed due to concurrent changes between Count and List.
	if count != nil {
		count.Add(total - processed)
	}

	drift, errE = b.diffMaterializedState(ctx)
	if errE != nil {
		return drift, errE
	}

	// Replace the tables' contents atomically, all three in one transaction, but only when something
	// differs, so a drift-free repair leaves the tables untouched.
	if !dryRun && drift.Any() {
		errE = b.replaceMaterializedState(ctx)
		if errE != nil {
			return drift, errE
		}
	}

	return drift, b.dropRepairStagingTables(ctx)
}

// recreateRepairStagingTables creates one empty unlogged staging table per materialized state table, with
// the same schema. The staging tables are unlogged because they are derived and rebuilt on any failure; a
// leftover from an interrupted run is dropped first.
func (b *Bridge) recreateRepairStagingTables(ctx context.Context) errors.E {
	return internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		for _, table := range materializedStateTables {
			_, err := tx.Exec(ctx, `DROP TABLE IF EXISTS "`+b.Store.Prefix+table+`Repair"`)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
			_, err = tx.Exec(ctx, `CREATE UNLOGGED TABLE "`+b.Store.Prefix+table+`Repair" (LIKE "`+b.Store.Prefix+table+`" INCLUDING ALL)`)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
		}
		return nil
	})
}

// dropRepairStagingTables drops the staging tables recreateRepairStagingTables created.
func (b *Bridge) dropRepairStagingTables(ctx context.Context) errors.E {
	return internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		for _, table := range materializedStateTables {
			_, err := tx.Exec(ctx, `DROP TABLE "`+b.Store.Prefix+table+`Repair"`)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
		}
		return nil
	})
}

// stageDocument computes the materialized state rows of one document at its latest version, the same way
// indexing a commit computes them, and appends them to rows. A deleted document contributes no rows.
func (b *Bridge) stageDocument(ctx context.Context, id identifier.Identifier, rows *stagedRows) errors.E {
	// Read the document once and produce its per-level versions through the indexing normalize hooks and
	// the source check.
	docs, sources, _, _, deleted, errE := b.produceLevels(ctx, id, nil)
	if errE != nil {
		return errE
	}
	if deleted {
		return nil
	}

	// The document's embed source paths are the union across visibility levels, one row per embedded-from
	// target (see the Embedding table).
	docEmbeds := map[identifier.Identifier]map[string][]identifier.Identifier{}

	for i, t := range b.targets {
		if docs[i] == nil {
			continue
		}
		ctxL := t.levelContext(ctx)

		outgoing, inverse, errE := t.Converter.OutgoingReferences(ctxL, docs[i])
		if errE != nil {
			errors.Details(errE)["id"] = id.String()
			return errE
		}
		// A claim reaching the same target through several value-hierarchy paths yields the row several
		// times, so rows are deduplicated per level the way the incremental diffing does.
		seenRefs := map[Reference]bool{}
		for targetID, targetRows := range outgoing {
			for _, row := range targetRows {
				row.IsSource = sources[i]
				if seenRefs[row] {
					continue
				}
				seenRefs[row] = true
				rows.RefTargets = append(rows.RefTargets, targetID.String())
				rows.RefLevels = append(rows.RefLevels, t.Level)
				rows.RefClaims = append(rows.RefClaims, row.Claim.String())
				rows.RefSources = append(rows.RefSources, row.Source.String())
				rows.RefIsSources = append(rows.RefIsSources, row.IsSource)
			}
		}
		// Inverse relations are kept only for documents which are sources at the level (see SourceCheck).
		if sources[i] {
			seenInverse := map[InverseRelation]bool{}
			for targetID, targetRows := range inverse {
				for _, row := range targetRows {
					if seenInverse[row] {
						continue
					}
					seenInverse[row] = true
					rows.InvTargets = append(rows.InvTargets, targetID.String())
					rows.InvLevels = append(rows.InvLevels, t.Level)
					rows.InvClaims = append(rows.InvClaims, row.Claim.String())
					rows.InvSources = append(rows.InvSources, row.Source.String())
					rows.InvTargetProps = append(rows.InvTargetProps, row.TargetProp.String())
					rows.InvSourceProps = append(rows.InvSourceProps, row.SourceProp.String())
					rows.InvConfidences = append(rows.InvConfidences, float64(row.Confidence))
				}
			}
		}

		outgoingEmbeds, errE := t.Converter.OutgoingEmbeds(docs[i])
		if errE != nil {
			errors.Details(errE)["id"] = id.String()
			return errE
		}
		mergeEmbeds(docEmbeds, outgoingEmbeds)
	}

	for targetID, pathSet := range docEmbeds {
		pathsJSON, errE := x.MarshalWithoutEscapeHTML(sortedPaths(pathSet))
		if errE != nil {
			errors.Details(errE)["id"] = id.String()
			return errE
		}
		rows.EmbTargets = append(rows.EmbTargets, targetID.String())
		rows.EmbEmbedders = append(rows.EmbEmbedders, id.String())
		rows.EmbPaths = append(rows.EmbPaths, string(pathsJSON))
	}

	return nil
}

// insertStagedRows inserts the staged rows into the staging tables. It is a no-op for an empty batch, so
// the caller can flush unconditionally.
func (b *Bridge) insertStagedRows(ctx context.Context, rows *stagedRows) errors.E {
	if rows.Len() == 0 {
		return nil
	}

	return internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		if len(rows.RefTargets) > 0 {
			_, err := tx.Exec(ctx, `
				INSERT INTO "`+b.Store.Prefix+`ReferencesRepair" ("target", "level", "claim", "source", "isSource")
					SELECT unnest($1::text[]), unnest($2::text[]), unnest($3::text[]), unnest($4::text[]), unnest($5::bool[])
			`, rows.RefTargets, rows.RefLevels, rows.RefClaims, rows.RefSources, rows.RefIsSources)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
		}
		if len(rows.InvTargets) > 0 {
			_, err := tx.Exec(ctx, `
				INSERT INTO "`+b.Store.Prefix+`InverseRelationsRepair" ("target", "level", "claim", "source", "targetProp", "sourceProp", "confidence")
					SELECT unnest($1::text[]), unnest($2::text[]), unnest($3::text[]), unnest($4::text[]),
						unnest($5::text[]), unnest($6::text[]), unnest($7::float8[])
			`, rows.InvTargets, rows.InvLevels, rows.InvClaims, rows.InvSources, rows.InvTargetProps, rows.InvSourceProps, rows.InvConfidences)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
		}
		if len(rows.EmbTargets) > 0 {
			_, err := tx.Exec(ctx, `
				INSERT INTO "`+b.Store.Prefix+`EmbeddingRepair" ("target", "embedder", "paths")
					SELECT unnest($1::text[]), unnest($2::text[]), unnest($3::jsonb[])
			`, rows.EmbTargets, rows.EmbEmbedders, rows.EmbPaths)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
		}
		return nil
	})
}

// diffMaterializedState diffs each materialized state table against its staging counterpart, in both
// directions, over whole rows.
func (b *Bridge) diffMaterializedState(ctx context.Context) (MaterializedStateDrift, errors.E) {
	drift := MaterializedStateDrift{}
	diffs := []struct {
		Table   string
		Extra   *int64
		Missing *int64
	}{
		{"References", &drift.ReferencesExtra, &drift.ReferencesMissing},
		{"InverseRelations", &drift.InverseRelationsExtra, &drift.InverseRelationsMissing},
		{"Embedding", &drift.EmbeddingExtra, &drift.EmbeddingMissing},
	}
	errE := internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Each diff reads every column of both tables and sorts or hashes them, which no index helps, so
		// the default statement timeout is lifted: it is exceeded somewhere above ten million rows per
		// table. A cancelled statement would not even fail cleanly, because RetryTransaction retries the
		// connection it is reported on.
		_, err := tx.Exec(ctx, `SET LOCAL statement_timeout = 0`)
		if err != nil {
			return internalStore.WithPgxError(err)
		}

		for _, diff := range diffs {
			// Initialize in the case transaction is retried.
			*diff.Extra = 0
			*diff.Missing = 0

			err = tx.QueryRow(ctx,
				`SELECT COUNT(*) FROM (TABLE "`+b.Store.Prefix+diff.Table+`" EXCEPT ALL TABLE "`+b.Store.Prefix+diff.Table+`Repair") AS "extra"`,
			).Scan(diff.Extra)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
			err = tx.QueryRow(ctx,
				`SELECT COUNT(*) FROM (TABLE "`+b.Store.Prefix+diff.Table+`Repair" EXCEPT ALL TABLE "`+b.Store.Prefix+diff.Table+`") AS "missing"`,
			).Scan(diff.Missing)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
		}
		return nil
	})
	return drift, errE
}

// replaceMaterializedState replaces the contents of every materialized state table with its staging
// counterpart's, all in one transaction.
func (b *Bridge) replaceMaterializedState(ctx context.Context) errors.E {
	return internalStore.RetryTransaction(ctx, b.dbpool, pgx.ReadWrite, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Every table is rewritten whole, together with its indexes, which exceeds the default statement
		// timeout from a few million rows on, so it is lifted. A cancelled statement would not even fail
		// cleanly, because RetryTransaction retries the connection it is reported on.
		_, err := tx.Exec(ctx, `SET LOCAL statement_timeout = 0`)
		if err != nil {
			return internalStore.WithPgxError(err)
		}

		for _, table := range materializedStateTables {
			_, err := tx.Exec(ctx, `TRUNCATE "`+b.Store.Prefix+table+`"`)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
			_, err = tx.Exec(ctx, `INSERT INTO "`+b.Store.Prefix+table+`" SELECT * FROM "`+b.Store.Prefix+table+`Repair"`)
			if err != nil {
				return internalStore.WithPgxError(err)
			}
		}
		return nil
	})
}
