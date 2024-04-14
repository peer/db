package store

import (
	"context"
	"strconv"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

type Version struct {
	Changeset identifier.Identifier
	Revision  int64
}

func (v Version) String() string {
	s := new(strings.Builder)
	s.WriteString(v.Changeset.String())
	s.WriteString("-")
	s.WriteString(strconv.FormatInt(v.Revision, 10))
	return s.String()
}

func (s *Store[Data, Metadata, Patch]) View(_ context.Context, view string) (View[Data, Metadata, Patch], errors.E) {
	// We do not check if the view exist at this point but only when we try to
	// get from the view, or commit to the view. Otherwise it would just be
	// racy as even if we check here it would not mean much until we really
	// try to use the view (view could disappear or be created in meantime).
	return View[Data, Metadata, Patch]{
		name:  view,
		store: s,
	}, nil
}

func (s *Store[Data, Metadata, Patch]) Insert(ctx context.Context, id identifier.Identifier, value Data, metadata Metadata) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	return view.Insert(ctx, id, value, metadata)
}

func (s *Store[Data, Metadata, Patch]) Replace(ctx context.Context, id, parentChangeset identifier.Identifier, value Data, metadata Metadata) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	return view.Replace(ctx, id, parentChangeset, value, metadata)
}

func (s *Store[Data, Metadata, Patch]) Update(
	ctx context.Context, id, parentChangeset identifier.Identifier, value Data, patch Patch, metadata Metadata,
) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	return view.Update(ctx, id, parentChangeset, value, patch, metadata)
}

func (s *Store[Data, Metadata, Patch]) Merge(
	ctx context.Context, id identifier.Identifier, parentChangesets []identifier.Identifier, value Data, patches []Patch, metadata Metadata,
) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	return view.Merge(ctx, id, parentChangesets, value, patches, metadata)
}

func (s *Store[Data, Metadata, Patch]) Delete(ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	return view.Delete(ctx, id, parentChangeset, metadata)
}

func (s *Store[Data, Metadata, Patch]) GetLatest(ctx context.Context, id identifier.Identifier) (Data, Metadata, Version, errors.E) { //nolint:ireturn
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return *new(Data), *new(Metadata), Version{}, errE //nolint:exhaustruct
	}
	return view.GetLatest(ctx, id)
}

func (s *Store[Data, Metadata, Patch]) Get(ctx context.Context, id identifier.Identifier, version Version) (Data, Metadata, errors.E) { //nolint:ireturn
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return *new(Data), *new(Metadata), errE
	}
	return view.Get(ctx, id, version)
}

func (s *Store[Data, Metadata, Patch]) Changeset(ctx context.Context, id identifier.Identifier) (Changeset[Data, Metadata, Patch], errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Changeset[Data, Metadata, Patch]{}, errE //nolint:exhaustruct
	}
	return view.Changeset(ctx, id)
}

func (s *Store[Data, Metadata, Patch]) Begin(ctx context.Context) (Changeset[Data, Metadata, Patch], errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Changeset[Data, Metadata, Patch]{}, errE //nolint:exhaustruct
	}
	return view.Begin(ctx)
}
