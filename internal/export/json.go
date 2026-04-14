package export

import (
	"context"
	"encoding/json"
	"io"
	"maps"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// JSONEntry holds structured claim data for JSON output.
type JSONEntry struct {
	ID      string
	Entries map[string][]any // Keyed by top-level prop name. Values: string or map[string]any.
}

// jsonVisitor implements document.Visitor to collect structured JSON entries.
type jsonVisitor struct {
	resolveName func(identifier.Identifier) string
	specs       []PropertySpec
	specStack   [][]PropertySpec
	path        []string
	entry       *JSONEntry
	subs        map[string]any
	subsStack   []map[string]any
}

var _ document.Visitor = (*jsonVisitor)(nil)

// recurse pushes the current specs, path, and subs, then visits sub-claims, then pops
// and returns the collected sub-values.
func (v *jsonVisitor) recurse(propName string, childSpecs []PropertySpec, claim document.Claim) (map[string]any, errors.E) {
	v.specStack = append(v.specStack, v.specs)
	v.specs = childSpecs
	v.path = append(v.path, propName)
	v.subsStack = append(v.subsStack, v.subs)
	v.subs = make(map[string]any)
	errE := claim.Visit(v)
	collected := v.subs
	v.subs = v.subsStack[len(v.subsStack)-1]
	v.subsStack = v.subsStack[:len(v.subsStack)-1]
	v.path = v.path[:len(v.path)-1]
	v.specs = v.specStack[len(v.specStack)-1]
	v.specStack = v.specStack[:len(v.specStack)-1]
	if len(collected) == 0 {
		return nil, errE
	}
	return collected, errE
}

// visitClaim is the shared logic for all JSON visitor methods.
func (v *jsonVisitor) visitClaim(propID identifier.Identifier, val string, isHas bool, claim document.Claim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}

	result := MatchAtDepth(propID, v.specs)
	if !result.Matched && len(result.ChildSpecs) == 0 {
		return document.Keep, nil
	}

	propName := v.resolveName(propID)

	// Try to collect sub-values first.
	var nested map[string]any
	if len(result.ChildSpecs) > 0 && claim.Size() > 0 {
		var errE errors.E
		nested, errE = v.recurse(propName, result.ChildSpecs, claim)
		if errE != nil {
			return document.Keep, errE
		}
	}

	// At top level (path is empty): create entries in v.entry.Entries.
	if len(v.path) == 0 {
		if isHas && claim.Size() == 0 {
			// Simple HasClaim without sub-claims: goes into __HAS__ column.
			v.entry.Entries[HasColumn] = append(v.entry.Entries[HasColumn], propName)
		} else if nested != nil {
			if result.Matched && val != "" {
				// Claim has both a value and nested sub-values.
				entryObj := map[string]any{"value": val}
				maps.Copy(entryObj, nested)
				v.entry.Entries[propName] = append(v.entry.Entries[propName], entryObj)
			} else {
				v.entry.Entries[propName] = append(v.entry.Entries[propName], nested)
			}
		} else if result.Matched && val != "" {
			v.entry.Entries[propName] = append(v.entry.Entries[propName], val)
		}
	} else {
		// Nested level: add to v.subs map.
		if !result.Matched {
			return document.Keep, nil
		}
		if nested != nil {
			if val != "" {
				entryObj := map[string]any{"value": val}
				maps.Copy(entryObj, nested)
				appendSubValue(v.subs, propName, entryObj)
			} else {
				appendSubValue(v.subs, propName, nested)
			}
		} else if val != "" {
			appendSubValue(v.subs, propName, val)
		}
	}

	return document.Keep, nil
}

// VisitIdentifier visits an identifier claim for JSON export.
func (v *jsonVisitor) VisitIdentifier(claim *document.IdentifierClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitString visits a string claim for JSON export.
func (v *jsonVisitor) VisitString(claim *document.StringClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitHTML visits an HTML claim for JSON export.
func (v *jsonVisitor) VisitHTML(claim *document.HTMLClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitAmount visits an amount claim for JSON export.
func (v *jsonVisitor) VisitAmount(claim *document.AmountClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitAmountInterval visits an amount interval claim for JSON export.
func (v *jsonVisitor) VisitAmountInterval(claim *document.AmountIntervalClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitTime visits a time claim for JSON export.
func (v *jsonVisitor) VisitTime(claim *document.TimeClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitTimeInterval visits a time interval claim for JSON export.
func (v *jsonVisitor) VisitTimeInterval(claim *document.TimeIntervalClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitLink visits a link claim for JSON export.
func (v *jsonVisitor) VisitLink(claim *document.LinkClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitReference visits a reference claim for JSON export.
func (v *jsonVisitor) VisitReference(claim *document.ReferenceClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitHas visits a has claim for JSON export.
func (v *jsonVisitor) VisitHas(claim *document.HasClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, "", true, claim)
}

// VisitNone visits a none claim for JSON export.
func (v *jsonVisitor) VisitNone(claim *document.NoneClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitUnknown visits an unknown claim for JSON export.
func (v *jsonVisitor) VisitUnknown(claim *document.UnknownClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// ProcessJSONDocument extracts structured entries from a document for JSON output.
func ProcessJSONDocument(ctx context.Context, doc *document.D, specs []PropertySpec, names *NameCache) JSONEntry {
	entry := JSONEntry{
		ID:      doc.ID.String(),
		Entries: make(map[string][]any),
	}
	v := &jsonVisitor{
		resolveName: func(propID identifier.Identifier) string {
			return names.DisplayName(ctx, propID)
		},
		specs:     specs,
		specStack: nil,
		path:      nil,
		entry:     &entry,
		subs:      nil,
		subsStack: nil,
	}
	// jsonVisitor never returns an error.
	_ = doc.Visit(v)
	return entry
}

// JSON exports documents as JSON-per-line. Single-pass: each document is written immediately.
func JSON(ctx context.Context, w io.Writer, docIDs []identifier.Identifier, specs []PropertySpec, names *NameCache, getDoc GetDocFunc) errors.E {
	enc := json.NewEncoder(w)

	for _, docID := range docIDs {
		doc, errE := getDoc(ctx, docID)
		if errE != nil {
			return errE
		}
		if doc == nil {
			continue
		}

		entry := ProcessJSONDocument(ctx, doc, specs, names)
		errE = WriteJSONEntry(enc, entry)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// WriteJSONEntry writes a single document as a JSON line.
// Claims with sub-claims are output as arrays of objects (e.g., [{"value": "byte", "IN_LANGUAGE": "..."}]).
// Claims without sub-claims are output as simple arrays (e.g., ["byte", "bajt"]).
func WriteJSONEntry(enc *json.Encoder, entry JSONEntry) errors.E {
	obj := make(map[string]any)
	obj["id"] = entry.ID

	for name, entries := range entry.Entries {
		if len(entries) == 0 {
			continue
		}

		// Check if any entry is a map (has sub-values).
		hasSubs := false
		for _, e := range entries {
			if _, ok := e.(map[string]any); ok {
				hasSubs = true
				break
			}
		}

		if hasSubs {
			// Output as array of objects.
			objs := make([]map[string]any, 0, len(entries))
			for _, e := range entries {
				switch v := e.(type) {
				case map[string]any:
					objs = append(objs, v)
				case string:
					// Simple string entry among entries that have subs.
					objs = append(objs, map[string]any{"value": v})
				default:
					objs = append(objs, map[string]any{"value": v})
				}
			}
			obj[name] = objs
		} else {
			// Output as simple array of strings.
			vals := make([]string, 0, len(entries))
			for _, e := range entries {
				if s, ok := e.(string); ok {
					vals = append(vals, s)
				}
			}
			obj[name] = vals
		}
	}

	err := enc.Encode(obj)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// appendSubValue adds a value to a sub-values map, converting to []any if the key already exists.
func appendSubValue(subs map[string]any, name string, val any) {
	if existing, ok := subs[name]; ok {
		if arr, ok := existing.([]any); ok {
			subs[name] = append(arr, val)
		} else {
			subs[name] = []any{existing, val}
		}
	} else {
		subs[name] = val
	}
}
