package store

import (
	"context"
	"strconv"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// Version represents a version of the value.
type Version struct {
	// Changeset is an ID of the changeset which contains the change of the value at this version.
	Changeset identifier.Identifier

	// Revision is a serial number of the change of the value at this version. It starts with 1.
	Revision int64
}

func (v Version) String() string {
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
		return Version{}, errors.Errorf("invalid version string: %s", text)
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

func (v Version) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

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
