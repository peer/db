package peerdb_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// TestRepair tests the flow of db repair against a store which was populated but never indexed: the
// metadata pass restores the extracted content (store.DocumentMetadata.InstanceOf) which was stripped
// (the state of a deployment predating its extraction), in place and preserving provenance, and the
// materialized state pass builds the references, inverse-relations, and embedding tables from the
// latest versions of the documents. The base is never started, matching how the command runs.
func TestRepair(t *testing.T) {
	t.Parallel()

	ctx, b, dbpool := initBaseInfra(t)

	// Populate the store without starting the base: the core documents and a pair of documents with a
	// DISTINCT_FROM claim between them.
	baseA := []string{"test", "repair", "A"}
	baseB := []string{"test", "repair", "B"}
	docB := &document.D{
		CoreDocument: document.CoreDocument{ID: identifier.From(baseB...), Base: baseB},
	}
	docA := distinctFromDoc(baseA, docB.ID)

	_, transformed, errE := base.GenerateCoreDocuments(ctx, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	transformed = append(transformed, docA, docB)
	for _, doc := range transformed {
		errE := b.InsertOrReplaceDocument(ctx, doc)
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	propertyFilter := json.RawMessage(`{"instanceOf":["` + internalCore.PropertyClassID.String() + `"]}`)

	// Strip the extracted content from every revision's metadata, the state of a deployment which
	// predates its extraction. Change rows are immutable in normal operation, so the strip lifts the
	// enforcing trigger for its transaction the way the repair itself does. Without the extracted
	// content nothing is listed by class.
	tx, err := dbpool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `ALTER TABLE "docsChanges" DISABLE TRIGGER "docsChangesNotAllowed"`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `UPDATE "docsChanges" SET "metadata" = "metadata" - 'instanceOf'`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `ALTER TABLE "docsChanges" ENABLE TRIGGER "docsChangesNotAllowed"`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	ids, errE := b.Documents().List(ctx, propertyFilter, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.Empty(t, ids)

	// The provenance and the version of a revision must survive the repair untouched.
	var atBefore string
	err = dbpool.QueryRow(ctx, `SELECT "metadata"->>'at' FROM "docsChanges" WHERE "id"=$1`, docA.ID.String()).Scan(&atBefore)
	require.NoError(t, err)
	_, _, versionBefore, _, errE := b.Documents().GetLatest(ctx, docA.ID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	var changesBefore int64
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM "docsChanges"`).Scan(&changesBefore)
	require.NoError(t, err)

	// A dry run reports the repairs without making them.
	examined, repaired, errE := b.RepairDocumentsMetadata(ctx, true, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, changesBefore, examined)
	assert.Positive(t, repaired)
	ids, errE = b.Documents().List(ctx, propertyFilter, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Empty(t, ids, "a dry run should not repair the metadata")

	// The repair restores the extracted content, in place.
	examinedAgain, repairedAgain, errE := b.RepairDocumentsMetadata(ctx, false, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, examined, examinedAgain)
	assert.Equal(t, repaired, repairedAgain)

	ids, errE = b.Documents().List(ctx, propertyFilter, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEmpty(t, ids)

	// The repair was made in place: same revisions, same versions, no new commits, and provenance intact.
	var atAfter string
	err = dbpool.QueryRow(ctx, `SELECT "metadata"->>'at' FROM "docsChanges" WHERE "id"=$1`, docA.ID.String()).Scan(&atAfter)
	require.NoError(t, err)
	assert.Equal(t, atBefore, atAfter)
	_, _, versionAfter, _, errE := b.Documents().GetLatest(ctx, docA.ID) //nolint:dogsled
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, versionBefore, versionAfter)
	var changesAfter int64
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*) FROM "docsChanges"`).Scan(&changesAfter)
	require.NoError(t, err)
	assert.Equal(t, changesBefore, changesAfter)

	// A further dry run finds nothing to repair.
	_, repairedMore, errE := b.RepairDocumentsMetadata(ctx, true, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Zero(t, repairedMore)

	// The materialized state pass builds the tables from the latest documents, with converters built
	// from the schema documents listed through the just-repaired metadata.
	converterDocs, errE := peerdb.TestingConverterDocuments(ctx, b)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.NotEmpty(t, converterDocs)

	drift, errE := b.RepairMaterializedState(ctx, converterDocs, false, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Positive(t, drift.ReferencesMissing)
	assert.Positive(t, drift.InverseRelationsMissing)
	assert.Zero(t, drift.ReferencesExtra)
	assert.Zero(t, drift.InverseRelationsExtra)

	// DISTINCT_FROM is its own inverse, so the pair's claim yields an inverse relation onto the target.
	var inverseRelations int64
	err = dbpool.QueryRow(ctx,
		`SELECT COUNT(*) FROM "docsInverseRelations" WHERE "target"=$1 AND "source"=$2`,
		docB.ID.String(), docA.ID.String(),
	).Scan(&inverseRelations)
	require.NoError(t, err)
	assert.Equal(t, int64(1), inverseRelations)

	// A further dry run reports no drift.
	drift, errE = b.RepairMaterializedState(ctx, converterDocs, true, nil, nil)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.False(t, drift.Any())
}
