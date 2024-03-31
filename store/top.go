package store

import (
	"context"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

type Version struct {
	Changeset identifier.Identifier
	Revision  int64
}

func (s *Store[Data, Metadata, Patch]) View(_ context.Context, view string) (View[Data, Metadata, Patch], errors.E) {
	// We do not check if the view exist at this point but only when we try to
	// get from the view, or commit to the view. Otherwise it would just be
	// racy as even if we check here it would not mean much until we really
	// try to use the view (view could disappear or be created in meantime).
	return View[Data, Metadata, Patch]{
		Name:  view,
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

func (s *Store[Data, Metadata, Patch]) Delete(ctx context.Context, id, parentChangeset identifier.Identifier, metadata Metadata) (Version, errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Version{}, errE //nolint:exhaustruct
	}
	return view.Delete(ctx, id, parentChangeset, metadata)
}

func (s *Store[Data, Metadata, Patch]) GetCurrent(ctx context.Context, id identifier.Identifier) (*Data, Metadata, Version, errors.E) { //nolint:ireturn
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return nil, *new(Metadata), Version{}, errE //nolint:exhaustruct
	}
	return view.GetCurrent(ctx, id)
}

func (s *Store[Data, Metadata, Patch]) Get(ctx context.Context, id identifier.Identifier, version Version) (*Data, Metadata, errors.E) { //nolint:ireturn
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return nil, *new(Metadata), errE
	}
	return view.Get(ctx, id, version)
}

func (s *Store[Data, Metadata, Patch]) Begin(ctx context.Context) (Changeset[Data, Metadata, Patch], errors.E) {
	view, errE := s.View(ctx, MainView)
	if errE != nil {
		return Changeset[Data, Metadata, Patch]{}, errE //nolint:exhaustruct
	}
	return view.Begin(ctx)
}
