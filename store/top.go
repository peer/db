package store

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	internal "gitlab.com/peerdb/peerdb/internal/store"
)

// Version represents a version of the value.
type Version struct {
	// Changeset is an ID of the changeset which contains the change of the value at this version.
	Changeset identifier.Identifier

	// Revision is a serial number of the change of the value at this version. It starts with 1.
	Revision int64
}

func (v *Version) String() string {
	s := new(strings.Builder)
	s.WriteString(v.Changeset.String())
	s.WriteString("-")
	s.WriteString(strconv.FormatInt(v.Revision, 10))
	return s.String()
}

// VersionFromString parses text as Version.
func VersionFromString(text string) (Version, errors.E) {
	changesetStr, revisionStr, ok := strings.Cut(text, "-")
	if !ok {
		errE := errors.New("invalid version string")
		errors.Details(errE)["value"] = text
		return Version{}, errE
	}
	changeset, errE := identifier.MaybeString(changesetStr)
	if errE != nil {
		return Version{}, errE
	}
	revision, err := strconv.ParseInt(revisionStr, 10, 64)
	if err != nil {
		return Version{}, errors.WithStack(err)
	}
	return Version{
		Changeset: changeset,
		Revision:  revision,
	}, errE
}

// MarshalText marshals a Version to text format.
func (v Version) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

// UnmarshalText unmarshals a Version from text format.
func (v *Version) UnmarshalText(text []byte) error {
	version, errE := VersionFromString(string(text))
	if errE != nil {
		return errE
	}
	*v = version
	return nil
}

// View returns a View instance for the provided name.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) View(
	_ context.Context, view string,
) (View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	// We do not check if the view exist at this point but only when we try to
	// get from the view, or commit to the view. Otherwise it would just be
	// racy as even if we check here it would not mean much until we really
	// try to use the view (view could disappear or be created in meantime).
	return View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
		name:  view,
		store: s,
	}, nil
}

// Insert auto-commits the insert change into the MainView.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Insert(
	ctx context.Context, id identifier.Identifier, value Data, metadata Metadata, commitMetadata CommitMetadata,
) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE
	}
	return view.Insert(ctx, id, value, metadata, commitMetadata)
}

// Replace auto-commits the replace change into the MainView.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Replace(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, metadata Metadata, commitMetadata CommitMetadata,
) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE
	}
	return view.Replace(ctx, id, parentChangeset, value, metadata, commitMetadata)
}

// Update auto-commits the update change into the MainView.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Update(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata, commitMetadata CommitMetadata,
) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE
	}
	return view.Update(ctx, id, parentChangeset, value, patch, metadata, commitMetadata)
}

// Merge auto-commits the merge change into the MainView.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Merge(
	ctx context.Context, id identifier.Identifier, parentChangesets []identifier.Identifier, value Data, patches []Patch, metadata Metadata, commitMetadata CommitMetadata,
) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE
	}
	return view.Merge(ctx, id, parentChangesets, value, patches, metadata, commitMetadata)
}

// Delete auto-commits the delete change into the MainView.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Delete(
	ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata, commitMetadata CommitMetadata,
) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE
	}
	return view.Delete(ctx, id, parentChangeset, metadata, commitMetadata)
}

// GetLatest returns the latest committed version of the value for the MainView.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) GetLatest( //nolint:ireturn
	ctx context.Context, id identifier.Identifier,
) (Data, Metadata, Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return *new(Data), *new(Metadata), Version{}, errE
	}
	return view.GetLatest(ctx, id)
}

// Get returns the value at a given version for the MainView.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Get( //nolint:ireturn
	ctx context.Context, id identifier.Identifier, version Version,
) (Data, Metadata, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return *new(Data), *new(Metadata), errE
	}
	return view.Get(ctx, id, version)
}

// Changeset returns the requested changeset.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Changeset(
	_ context.Context, id identifier.Identifier,
) (Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	// We do not care if the changeset exists at this point. It all
	// depends what we will be doing with it and we do checks then.
	return Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
		id:    id,
		store: s,
	}, nil
}

// Begin starts a new changeset.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Begin(
	ctx context.Context,
) (Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	return s.Changeset(ctx, identifier.New())
}

// Commit commits a changeset to the MainView.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Commit(
	ctx context.Context, changeset Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], metadata CommitMetadata,
) ([]Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return nil, errE
	}
	return view.Commit(ctx, changeset, metadata)
}

// List returns up to MaxPageLength value IDs committed to the MainView, ordered by ID, after optional ID, to support keyset pagination.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) List(
	ctx context.Context, after *identifier.Identifier,
) ([]identifier.Identifier, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return nil, errE
	}
	return view.List(ctx, after)
}

// Changes returns up to MaxPageLength changesets for the value committed to the MainView, ordered first by depth
// in increasing order (newest changes first) and then by changeset ID, after optional changeset ID, to
// support keyset pagination.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) Changes(
	ctx context.Context, id identifier.Identifier, after *identifier.Identifier,
) ([]identifier.Identifier, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return nil, errE
	}
	return view.Changes(ctx, id, after)
}

// CommitLog returns up to MaxPageLength commit log entries in increasing seq order, after optional
// seq number, to support keyset pagination. The optional view parameter filters results to commits
// whose view has that name.
//
// The changesets and views in the returned CommittedChangesets do not have an associated Store.
func (s *Store[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]) CommitLog(
	ctx context.Context, after *int64, view *string,
) ([]CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], errors.E) {
	var conditions []string
	var arguments []any
	if after != nil {
		arguments = append(arguments, *after)
		conditions = append(conditions, `cl."seq" > $`+strconv.Itoa(len(arguments)))
	}
	if view != nil {
		arguments = append(arguments, *view)
		conditions = append(conditions, `cv."name" = $`+strconv.Itoa(len(arguments)))
	}
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}
	var commits []CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]
	errE := internal.RetryTransaction(ctx, s.dbpool, pgx.ReadOnly, func(ctx context.Context, tx pgx.Tx) errors.E {
		// Initialize in the case transaction is retried.
		commits = make([]CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], 0, MaxPageLength)

		// Join with CurrentViews to get the current name of the view, not the name stored at commit time.
		rows, err := tx.Query(ctx, `
			SELECT cl."seq", cv."name", cl."changesets"
				FROM "`+s.Prefix+`CommitLog" cl
					JOIN "`+s.Prefix+`CurrentViews" cv USING ("view")
				`+whereClause+`
				ORDER BY cl."seq" ASC
				LIMIT `+maxPageLengthStr, arguments...)
		if err != nil {
			return internal.WithPgxError(err)
		}
		var seq int64
		var name *string
		var changesets []string
		_, err = pgx.ForEachRow(rows, []any{&seq, &name, &changesets}, func() error {
			viewName := ""
			if name != nil {
				// cv."name" may be NULL if the view has since been released (removed).
				viewName = *name
			}
			commit := CommittedChangesets[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
				Seq:        seq,
				Changesets: make([]Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch], 0, len(changesets)),
				View: View[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
					name:  viewName,
					store: nil,
				},
			}
			for _, changesetID := range changesets {
				commit.Changesets = append(commit.Changesets, Changeset[Data, Metadata, CreateViewMetadata, ReleaseViewMetadata, CommitMetadata, Patch]{
					id:    identifier.String(changesetID),
					store: nil,
				})
			}
			commits = append(commits, commit)
			return nil
		})
		if err != nil {
			return internal.WithPgxError(err)
		}
		return nil
	})
	if errE != nil {
		if after != nil {
			errors.Details(errE)["after"] = *after
		}
		if view != nil {
			errors.Details(errE)["view"] = *view
		}
	}
	return commits, errE
}
