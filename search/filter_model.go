package search

import (
	"slices"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// ToValue represents a target value in a reference filter.
type ToValue struct {
	ID identifier.Identifier `json:"id"`
}

// HasValue represents a selected property value in a has filter.
type HasValue struct {
	ID identifier.Identifier `json:"id"`
}

// RefFilter contains the selected values of a reference filter.
//
// Direct holds values selected through their "direct" option: a value in Direct matches only
// documents for which the value is most-specific (it references the value but none of its narrower
// values). It parallels To (which matches the value and all its narrower values) and is OR-ed with
// To and with the path's specials.
type RefFilter struct {
	To     []ToValue `json:"to,omitempty"`
	Direct []ToValue `json:"direct,omitempty"`
}

// Validate validates the RefFilter.
func (f *RefFilter) Validate() errors.E {
	if len(f.To) == 0 && len(f.Direct) == 0 {
		return errors.New("to or direct has to be set")
	}
	return nil
}

// AmountFilter contains the selected range of an amount filter.
//
// Exists matches documents which have the property (with the filter's unit) with an actual value
// record, whatever its bounds. It is disjoint from the path's unknown special (a value stated to
// exist without being known); a user wanting either selects both.
type AmountFilter struct {
	Unit   *identifier.Identifier `json:"unit,omitempty"`
	Gte    *float64               `json:"gte,omitempty"`
	Lte    *float64               `json:"lte,omitempty"`
	Exists bool                   `json:"exists,omitempty"`
}

// Validate validates the AmountFilter.
func (f *AmountFilter) Validate() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.Exists {
		return errors.New("gte and lte, or exists has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Exists {
		return errors.New("gte/lte and exists cannot be both set")
	}
	if (f.Gte == nil) != (f.Lte == nil) {
		return errors.New("both gte and lte must be set together")
	}
	return nil
}

// TimeFilter contains the selected range of a time filter.
//
// Gte and Lte are in seconds since Unix epoch.
//
// Exists matches documents which have the property with an actual value record, whatever its
// bounds. It is disjoint from the path's unknown special; a user wanting either selects both.
type TimeFilter struct {
	Gte    *float64 `json:"gte,omitempty"`
	Lte    *float64 `json:"lte,omitempty"`
	Exists bool     `json:"exists,omitempty"`
}

// Validate validates the TimeFilter.
func (f *TimeFilter) Validate() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.Exists {
		return errors.New("gte and lte, or exists has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Exists {
		return errors.New("gte/lte and exists cannot be both set")
	}
	if (f.Gte == nil) != (f.Lte == nil) {
		return errors.New("both gte and lte must be set together")
	}
	return nil
}

// HasFilter contains the selected properties of a pooled has facet.
//
// Props lists the has-property IDs the filter matches against. The values are OR'd together: a
// document matches the filter when any of the listed properties is present as a has claim, at the
// top level (empty Prop) or under a parent property (one-element Prop). Properties whose has
// claims migrated to their own facet select through that facet's "has property" special instead,
// but a selection made through the pooled facet keeps compiling identically either way.
type HasFilter struct {
	Props []HasValue `json:"props,omitempty"`
}

// Validate validates the HasFilter.
func (f *HasFilter) Validate() errors.E {
	if len(f.Props) == 0 {
		return errors.New("props has to be set")
	}
	return nil
}

// SpecialsFilter contains the selected special values of a property path. The specials are facts
// about the path, not about one value-type facet, so they are stored once per path (per filter
// set) and rendered identically in every value-type facet of that path. They OR with the path's
// valued selections.
//
// Missing selects documents whose facet universe contains them but which state nothing facetable
// for the path: at the top level, no rel, amount, or time claim for the property; at the sub
// level, a parent claim exists but no facetable sub-claim for the property is under any parent
// claim (matching the parent constraints). None, Unknown, and HasProperty select documents with a
// none, unknown, or has statement for the path; unlike Missing they are positive per-claim
// conditions and correlate inside groups like values do.
type SpecialsFilter struct {
	Missing     bool `json:"missing,omitempty"`
	None        bool `json:"none,omitempty"`
	Unknown     bool `json:"unknown,omitempty"`
	HasProperty bool `json:"hasProperty,omitempty"`
}

// Any reports whether any special is selected.
func (f *SpecialsFilter) Any() bool {
	return f != nil && (f.Missing || f.None || f.Unknown || f.HasProperty)
}

// Validate validates the SpecialsFilter.
func (f *SpecialsFilter) Validate() errors.E {
	if !f.Any() {
		return errors.New("at least one of missing, none, unknown, or hasProperty has to be set")
	}
	return nil
}

// Filter represents a single active search filter.
//
// Exactly one of Ref, Amount, Time, Has, or Specials must be set. Prop is the filter's property
// path: one element for a top-level property, two for a sub-property under a parent (Prop[0] is
// the parent claim's property, Prop[1] the sub-claim's property). The Has filter is the pooled has
// facet: it takes no Prop at the top level and a single Prop element (the parent property) in its
// sub form, with HasFilter.Props selecting the has-properties to match. The Specials filter takes
// a one- or two-element Prop and holds the path's special-value selections; at most one specials
// filter per path may exist in a filter set.
type Filter struct {
	ID       *identifier.Identifier  `json:"id,omitempty"`
	Base     []string                `json:"base,omitempty"`
	Prop     []identifier.Identifier `json:"prop"`
	Ref      *RefFilter              `json:"ref,omitempty"`
	Amount   *AmountFilter           `json:"amount,omitempty"`
	Time     *TimeFilter             `json:"time,omitempty"`
	Has      *HasFilter              `json:"has,omitempty"`
	Specials *SpecialsFilter         `json:"specials,omitempty"`
}

// Validate validates the Filter to ensure it has a valid configuration.
func (f Filter) Validate(withoutSession bool) errors.E {
	if !withoutSession {
		if len(f.Base) < 2 { //nolint:mnd
			errE := errors.New("base must have at least two elements")
			errors.Details(errE)["length"] = len(f.Base)
			return errE
		}

		expectedID := identifier.From(f.Base...)
		if f.ID == nil || *f.ID != expectedID {
			errE := errors.New("invalid filter ID")
			errors.Details(errE)["got"] = f.ID.String()
			errors.Details(errE)["expected"] = expectedID.String()
			return errE
		}
	} else {
		if len(f.Base) > 0 {
			errE := errors.New("base must be empty")
			errors.Details(errE)["length"] = len(f.Base)
			return errE
		}
		if f.ID != nil {
			errE := errors.New("id must be empty")
			errors.Details(errE)["id"] = f.ID.String()
			return errE
		}
	}

	nonEmpty := 0
	if f.Ref != nil {
		nonEmpty++
	}
	if f.Amount != nil {
		nonEmpty++
	}
	if f.Time != nil {
		nonEmpty++
	}
	if f.Has != nil {
		nonEmpty++
	}
	if f.Specials != nil {
		nonEmpty++
	}
	if nonEmpty != 1 {
		return errors.New("exactly one of ref, amount, time, has, or specials must be set")
	}

	// Ref, Amount, Time, and Specials filters take one prop at top level (the claim's property)
	// and two props in their sub form (parentProp + prop). The Has filter takes no prop at top
	// level and a single prop (the parent property) in its sub form; HasFilter.Props selects
	// which has-properties to match.
	switch {
	case f.Has != nil:
		if len(f.Prop) > 1 {
			errE := errors.New("prop must have zero or one elements for has filter")
			errors.Details(errE)["length"] = len(f.Prop)
			return errE
		}
	default:
		if len(f.Prop) != 1 && len(f.Prop) != 2 {
			errE := errors.New("prop must have one or two elements")
			errors.Details(errE)["length"] = len(f.Prop)
			return errE
		}
	}

	if f.Ref != nil {
		return f.Ref.Validate()
	}
	if f.Amount != nil {
		return f.Amount.Validate()
	}
	if f.Time != nil {
		return f.Time.Validate()
	}
	if f.Has != nil {
		return f.Has.Validate()
	}
	return f.Specials.Validate()
}

// GetFilterByID finds a filter by ID in the session's filters.
func (s *Session) GetFilterByID(id identifier.Identifier) (*Filter, errors.E) {
	for i := range s.Filters {
		if s.Filters[i].ID != nil && *s.Filters[i].ID == id {
			return &s.Filters[i], nil
		}
	}
	return nil, errors.WithDetails(ErrNotFound, "filter", id)
}

// pathString renders a filter's property path as a stable string key.
func pathString(prop []identifier.Identifier) string {
	out := ""
	var outSb255 strings.Builder
	for i, p := range prop {
		if i > 0 {
			outSb255.WriteString("/")
		}
		outSb255.WriteString(p.String())
	}
	out += outSb255.String()
	return out
}

// validateFilters validates each filter in filters and records its ID in seen to detect
// duplicates, and each specials filter's path in seenSpecials to enforce at most one specials
// filter per path within the set. field is the error detail key identifying the set ("filter" or
// "prefilter").
func validateFilters(filters []Filter, field string, withoutSession bool, seen map[identifier.Identifier]bool, seenSpecials map[string]bool) errors.E {
	for i, f := range filters {
		errE := f.Validate(withoutSession)
		if errE != nil {
			errors.Details(errE)[field] = i
			return errE
		}
		if f.Specials != nil {
			key := pathString(f.Prop)
			if seenSpecials[key] {
				errE := errors.New("duplicate specials filter for path")
				errors.Details(errE)["path"] = key
				errors.Details(errE)[field] = i
				return errE
			}
			seenSpecials[key] = true
		}
		if !withoutSession {
			// We checked that f.ID is not nil in f.Validate().
			if seen[*f.ID] {
				errE := errors.New("duplicate filter ID")
				errors.Details(errE)["id"] = f.ID.String()
				errors.Details(errE)[field] = i
				return errE
			}
			seen[*f.ID] = true
		}
	}
	return nil
}

// SpecialsFor returns the specials selection active for the given property path in the session's
// filters (not prefilters), or nil when none is. Filters whose ID is in excludeIDs are skipped.
func (s *SessionData) SpecialsFor(prop []identifier.Identifier, excludeIDs ...identifier.Identifier) *SpecialsFilter {
	for i := range s.Filters {
		f := &s.Filters[i]
		if f.Specials == nil || !SamePropPath(f.Prop, prop) {
			continue
		}
		if f.ID != nil && idInList(*f.ID, excludeIDs) {
			continue
		}
		return f.Specials
	}
	return nil
}

// SamePropPath reports whether two property paths are equal.
func SamePropPath(a, b []identifier.Identifier) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// idInList reports whether id is one of ids.
func idInList(id identifier.Identifier, ids []identifier.Identifier) bool {
	return slices.Contains(ids, id)
}
