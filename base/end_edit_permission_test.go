package base_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/auth"
	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/coordinator"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/store"
)

// TestDocumentSessionEndEditPermissionCheck exercises EndEditPermissionCheck: a rejected completion
// (a commit or a discard) completes the session as errored without committing anything, a session
// ended through a context marked with WithSystemSession skips the check, a session begun through
// such a context has to be ended through one as well, and the check receives the user and roles
// recorded when the session was ended.
func TestDocumentSessionEndEditPermissionCheck(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	type endCheck struct {
		user  *store.User
		roles []string
	}
	var mu sync.Mutex
	var checks []endCheck
	allow := false
	b.EndEditPermissionCheck = func(user *store.User, roles []string, _ *document.D) errors.E {
		mu.Lock()
		defer mu.Unlock()
		checks = append(checks, endCheck{user, roles})
		if allow {
			return nil
		}
		return errors.New("completion rejected")
	}

	// appendCtx is the context the change is appended with: a session the application owns has to be
	// appended to with a system context, like it has to be ended with one.
	appendOneChange := func(appendCtx context.Context, docBase []string, session identifier.Identifier) {
		confidence := document.HighConfidence
		propID := identifier.New()
		changeBase := append(append([]string{}, docBase...), "SESSION", session.String(), "1")
		changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
			ID:   identifier.From(changeBase...),
			Base: changeBase,
			Patch: document.StringClaimPatch{
				Confidence: &confidence,
				Prop:       &propID,
				String:     "value",
			},
		})
		_, errE := b.AppendDocumentChange(appendCtx, session, changeJSON, 1)
		require.NoError(t, errE, "% -+#.1v", errE)
	}

	// A rejected create session completes as errored and nothing is committed.
	docID, docBase := newDocID()
	session, errE := b.BeginCreateDocument(ctx, docBase)
	require.NoError(t, errE, "% -+#.1v", errE)
	appendOneChange(ctx, docBase, session)
	endCtx := auth.WithRoles(auth.WithSubject(ctx, "committer"), []string{"tester"})
	errE = b.EndEditDocument(endCtx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.True(c, completeMetadata.Errored)
			assert.False(c, completeMetadata.Discarded)
			assert.Nil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)
	_, _, _, _, errE = b.GetDocumentLatest(ctx, docID) //nolint:dogsled
	require.ErrorIs(t, errE, store.ErrValueNotFound)

	// A completed session has no current document state anymore.
	_, _, errE = b.SessionDocumentRaw(ctx, session)
	require.ErrorIs(t, errE, coordinator.ErrAlreadyCompleted)

	mu.Lock()
	require.Len(t, checks, 1)
	require.NotNil(t, checks[0].user)
	assert.Equal(t, "committer", checks[0].user.ID)
	assert.Equal(t, []string{"tester"}, checks[0].roles)
	mu.Unlock()

	// A session ended through a context marked with WithSystemSession skips the check and commits.
	docID, docBase = newDocID()
	session, errE = b.BeginCreateDocument(ctx, docBase)
	require.NoError(t, errE, "% -+#.1v", errE)
	appendOneChange(ctx, docBase, session)
	errE = b.EndEditDocument(base.WithSystemSession(ctx), session, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.False(c, completeMetadata.Errored)
			assert.False(c, completeMetadata.Discarded)
			assert.NotNil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)
	mu.Lock()
	assert.Len(t, checks, 1)
	mu.Unlock()

	// A session begun through a context marked with WithSystemSession is the application's own and
	// has to be ended through one as well: ending it with an ordinary context is a programming error,
	// so the completion is rejected (the session completes as errored, nothing is committed) instead
	// of being checked against whoever ended it.
	systemDocID, systemDocBase := newDocID()
	systemSession, errE := b.BeginCreateDocument(base.WithSystemSession(ctx), systemDocBase)
	require.NoError(t, errE, "% -+#.1v", errE)
	appendOneChange(base.WithSystemSession(ctx), systemDocBase, systemSession)
	errE = b.EndEditDocument(ctx, systemSession, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, completeMetadata, errE := b.GetEditDocumentSession(ctx, systemSession)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.True(c, completeMetadata.Errored)
			assert.Nil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)
	_, _, _, _, errE = b.GetDocumentLatest(ctx, systemDocID) //nolint:dogsled
	require.ErrorIs(t, errE, store.ErrValueNotFound)
	// The check never ran: the completion was rejected before it.
	mu.Lock()
	assert.Len(t, checks, 1)
	mu.Unlock()

	// An allowed edit session of the just created document commits, and the check sees an edit
	// session ended by an unauthenticated caller.
	mu.Lock()
	allow = true
	mu.Unlock()
	session, _, errE = beginEditDocumentLatest(ctx, b, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	appendOneChange(ctx, docBase, session)
	errE = b.EndEditDocument(ctx, session, false)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.False(c, completeMetadata.Errored)
			assert.NotNil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)
	mu.Lock()
	require.Len(t, checks, 2)
	assert.Nil(t, checks[1].user)
	mu.Unlock()

	// The check runs also for a discarded session with changes: an allowed discard completes as
	// discarded.
	session, _, errE = beginEditDocumentLatest(ctx, b, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	appendOneChange(ctx, docBase, session)
	errE = b.EndEditDocument(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.False(c, completeMetadata.Errored)
			assert.True(c, completeMetadata.Discarded)
			assert.Nil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)
	mu.Lock()
	require.Len(t, checks, 3)
	allow = false
	mu.Unlock()

	// A rejected discard completes the session as errored: the session still ends (the end is
	// already recorded when the check runs), but the completion is recorded as not allowed instead
	// of as a clean discard, and nothing is committed.
	session, _, errE = beginEditDocumentLatest(ctx, b, docID)
	require.NoError(t, errE, "% -+#.1v", errE)
	appendOneChange(ctx, docBase, session)
	errE = b.EndEditDocument(ctx, session, true)
	require.NoError(t, errE, "% -+#.1v", errE)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, _, completeMetadata, errE := b.GetEditDocumentSession(ctx, session)
		if !assert.NoError(c, errE, "% -+#.1v", errE) {
			return
		}
		if assert.NotNil(c, completeMetadata) {
			assert.True(c, completeMetadata.Errored)
			assert.Nil(c, completeMetadata.Changeset)
		}
	}, 30*time.Second, 100*time.Millisecond)
	mu.Lock()
	require.Len(t, checks, 4)
	mu.Unlock()
}

// beginEditDocumentLatest begins an edit session of the latest version of the document.
func beginEditDocumentLatest(ctx context.Context, b *base.B, id identifier.Identifier) (identifier.Identifier, store.Version, errors.E) {
	doc, _, version, _, errE := b.GetDocumentLatestDoc(ctx, id)
	if errE != nil {
		return identifier.Identifier{}, store.Version{}, errE
	}
	session, errE := b.BeginEditDocument(ctx, version, doc)
	return session, version, errE
}

// TestDocumentSessionChangePermissionCheck exercises ChangePermissionCheck: a rejected change is not
// appended, the check receives the states before and after the change, changes appended through a
// context marked with WithSystemSession skip the check, and a session begun through such a context
// has to be appended to through one as well.
func TestDocumentSessionChangePermissionCheck(t *testing.T) {
	t.Parallel()

	ctx, b := initBase(t)

	propID := identifier.New()

	var mu sync.Mutex
	allow := true
	beforeCount := -1
	afterCount := -1
	b.ChangePermissionCheck = func(_ context.Context, before, after *document.D, _ document.Change) errors.E {
		mu.Lock()
		defer mu.Unlock()
		beforeCount = len(before.Get(propID))
		afterCount = len(after.Get(propID))
		if allow {
			return nil
		}
		return errors.New("change rejected")
	}

	_, docBase := newDocID()
	session, errE := b.BeginCreateDocument(ctx, docBase)
	require.NoError(t, errE, "% -+#.1v", errE)

	appendChange := func(ctx context.Context, seqNo int64) errors.E {
		confidence := document.HighConfidence
		changeBase := append(append([]string{}, docBase...), "SESSION", session.String(), strconv.FormatInt(seqNo, 10))
		changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
			ID:   identifier.From(changeBase...),
			Base: changeBase,
			Patch: document.StringClaimPatch{
				Confidence: &confidence,
				Prop:       &propID,
				String:     "value",
			},
		})
		_, errE := b.AppendDocumentChange(ctx, session, changeJSON, seqNo)
		return errE
	}

	// An allowed change is appended, with the states before and after the change passed to the check.
	errE = appendChange(ctx, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
	mu.Lock()
	assert.Equal(t, 0, beforeCount)
	assert.Equal(t, 1, afterCount)
	allow = false
	mu.Unlock()

	// A rejected change is not appended.
	errE = appendChange(ctx, 2)
	require.EqualError(t, errE, "change rejected")
	last, errE := b.LastDocumentChange(ctx, session)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, int64(1), last)

	// A change appended through a system session context skips the check.
	errE = appendChange(base.WithSystemSession(ctx), 2)
	require.NoError(t, errE, "% -+#.1v", errE)

	// A session begun through a system session context is the application's own and has to be
	// appended to through one as well: appending with an ordinary context is a programming error and
	// is rejected, rather than let through unchecked.
	_, systemDocBase := newDocID()
	systemSession, errE := b.BeginCreateDocument(base.WithSystemSession(ctx), systemDocBase)
	require.NoError(t, errE, "% -+#.1v", errE)
	confidence := document.HighConfidence
	changeBase := append(append([]string{}, systemDocBase...), "SESSION", systemSession.String(), "1")
	changeJSON := marshalChange(t, document.AddClaimChange{ //nolint:exhaustruct
		ID:   identifier.From(changeBase...),
		Base: changeBase,
		Patch: document.StringClaimPatch{
			Confidence: &confidence,
			Prop:       &propID,
			String:     "value",
		},
	})
	_, errE = b.AppendDocumentChange(ctx, systemSession, changeJSON, 1)
	assert.EqualError(t, errE, "system session appended to without a system context")
	_, errE = b.AppendDocumentChange(base.WithSystemSession(ctx), systemSession, changeJSON, 1)
	require.NoError(t, errE, "% -+#.1v", errE)
}
