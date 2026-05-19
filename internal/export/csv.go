package export

import (
	"context"
	"encoding/csv"
	"io"
	"slices"
	"strings"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// Column represents a single output column.
type Column struct {
	Path  []string // Display name path segments.
	IsHas bool
}

// ColumnKey returns the internal map key for a column path.
func ColumnKey(path []string) string {
	return strings.Join(path, "\x00")
}

// CSVName returns the dot-separated display name for CSV output.
func (c Column) CSVName() string {
	return strings.Join(c.Path, ".")
}

// CSVRow holds flat column values for CSV output.
type CSVRow struct {
	ID     string
	Values map[string][]string // Keyed by ColumnKey(path).
}

// csvVisitor implements document.Visitor to collect flat CSV column values.
type csvVisitor struct {
	resolveName func(identifier.Identifier) string
	specs       []PropertySpec
	specStack   [][]PropertySpec
	path        []string
	row         *CSVRow
	columnSet   map[string]Column
}

var _ document.Visitor = (*csvVisitor)(nil)

// recurse pushes the current specs and path, then visits sub-claims, then pops.
func (v *csvVisitor) recurse(propName string, childSpecs []PropertySpec, claim document.Claim) errors.E {
	v.specStack = append(v.specStack, v.specs)
	v.specs = childSpecs
	v.path = append(v.path, propName)
	errE := claim.Visit(v)
	v.path = v.path[:len(v.path)-1]
	v.specs = v.specStack[len(v.specStack)-1]
	v.specStack = v.specStack[:len(v.specStack)-1]
	return errE
}

// visitClaim is the shared logic for all CSV visitor methods.
func (v *csvVisitor) visitClaim(propID identifier.Identifier, val string, isHas bool, claim document.Claim) (document.VisitResult, errors.E) {
	if claim.GetConfidence() < document.LowConfidence {
		return document.Keep, nil
	}

	result := MatchAtDepth(propID, v.specs)
	if !result.Matched && len(result.ChildSpecs) == 0 {
		return document.Keep, nil
	}

	propName := v.resolveName(propID)

	if result.Matched {
		if isHas && claim.Size() == 0 {
			// Simple HasClaim without sub-claims: goes into __HAS__ column.
			hasPath := []string{HasColumn}
			key := ColumnKey(hasPath)
			v.columnSet[key] = Column{Path: hasPath, IsHas: true}
			v.row.Values[key] = append(v.row.Values[key], propName)
		} else if val != "" {
			path := append(slices.Clone(v.path), propName)
			key := ColumnKey(path)
			v.columnSet[key] = Column{Path: path, IsHas: false}
			v.row.Values[key] = append(v.row.Values[key], val)
		}
	}

	if len(result.ChildSpecs) > 0 && claim.Size() > 0 {
		errE := v.recurse(propName, result.ChildSpecs, claim)
		if errE != nil {
			return document.Keep, errE
		}
	}

	return document.Keep, nil
}

// VisitIdentifier visits an identifier claim for CSV export.
func (v *csvVisitor) VisitIdentifier(claim *document.IdentifierClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitString visits a string claim for CSV export.
func (v *csvVisitor) VisitString(claim *document.StringClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitHTML visits an HTML claim for CSV export.
func (v *csvVisitor) VisitHTML(claim *document.HTMLClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitAmount visits an amount claim for CSV export.
func (v *csvVisitor) VisitAmount(claim *document.AmountClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitAmountInterval visits an amount interval claim for CSV export.
func (v *csvVisitor) VisitAmountInterval(claim *document.AmountIntervalClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitTime visits a time claim for CSV export.
func (v *csvVisitor) VisitTime(claim *document.TimeClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitTimeInterval visits a time interval claim for CSV export.
func (v *csvVisitor) VisitTimeInterval(claim *document.TimeIntervalClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitLink visits a link claim for CSV export.
func (v *csvVisitor) VisitLink(claim *document.LinkClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitReference visits a reference claim for CSV export.
func (v *csvVisitor) VisitReference(claim *document.ReferenceClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitHas visits a has claim for CSV export.
func (v *csvVisitor) VisitHas(claim *document.HasClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, "", true, claim)
}

// VisitNone visits a none claim for CSV export.
func (v *csvVisitor) VisitNone(claim *document.NoneClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// VisitUnknown visits an unknown claim for CSV export.
func (v *csvVisitor) VisitUnknown(claim *document.UnknownClaim) (document.VisitResult, errors.E) {
	return v.visitClaim(claim.Prop.ID, ClaimValue(claim), false, claim)
}

// ProcessCSVDocument extracts flat column values from a document for CSV output.
func ProcessCSVDocument(ctx context.Context, doc *document.D, specs []PropertySpec, names *NameCache, columnSet map[string]Column) CSVRow {
	row := CSVRow{
		ID:     doc.ID.String(),
		Values: make(map[string][]string),
	}
	v := &csvVisitor{
		resolveName: func(propID identifier.Identifier) string {
			return names.DisplayName(ctx, propID)
		},
		specs:     specs,
		specStack: nil,
		path:      nil,
		row:       &row,
		columnSet: columnSet,
	}
	// csvVisitor never returns an error.
	_ = doc.Visit(v)
	return row
}

// SortColumns sorts columns alphabetically with __HAS__ at the end.
func SortColumns(columnSet map[string]Column) []Column {
	columns := make([]Column, 0, len(columnSet))
	for _, col := range columnSet {
		columns = append(columns, col)
	}
	slices.SortFunc(columns, func(a, b Column) int {
		if a.IsHas != b.IsHas {
			if a.IsHas {
				return 1
			}
			return -1
		}
		return slices.Compare(a.Path, b.Path)
	})
	return columns
}

// CSV exports documents as CSV. Two-pass: first discovers columns, then writes rows.
func CSV(ctx context.Context, w io.Writer, docIDs []identifier.Identifier, specs []PropertySpec, names *NameCache, getDoc GetDocFunc) errors.E {
	// Pass 1: Discover columns.
	columnSet := make(map[string]Column)
	for _, docID := range docIDs {
		doc, errE := getDoc(ctx, docID)
		if errE != nil {
			return errE
		}
		if doc == nil {
			continue
		}
		// Process document to discover columns, discard row data.
		ProcessCSVDocument(ctx, doc, specs, names, columnSet)
	}

	columns := SortColumns(columnSet)

	// Pass 2: Write header and stream rows.
	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := make([]string, 0, len(columns)+1)
	header = append(header, "id")
	for _, col := range columns {
		header = append(header, col.CSVName())
	}
	err := cw.Write(header)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, docID := range docIDs {
		doc, errE := getDoc(ctx, docID)
		if errE != nil {
			return errE
		}
		if doc == nil {
			continue
		}

		row := ProcessCSVDocument(ctx, doc, specs, names, columnSet)
		errE = WriteCSVRow(cw, columns, row)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// WriteCSVRow writes a single document's data as one or more CSV rows.
func WriteCSVRow(cw *csv.Writer, columns []Column, row CSVRow) errors.E {
	// Find max repetition count across all columns.
	maxReps := 1
	for _, col := range columns {
		key := ColumnKey(col.Path)
		if len(row.Values[key]) > maxReps {
			maxReps = len(row.Values[key])
		}
	}

	for rep := range maxReps {
		record := make([]string, 0, len(columns)+1)
		if rep == 0 {
			record = append(record, row.ID)
		} else {
			record = append(record, "")
		}
		for _, col := range columns {
			vals := row.Values[ColumnKey(col.Path)]
			if rep < len(vals) {
				record = append(record, vals[rep])
			} else {
				record = append(record, "")
			}
		}
		err := cw.Write(record)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}
