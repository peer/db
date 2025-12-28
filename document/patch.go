package document

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

func idAtIndex(base identifier.Identifier, i int64) identifier.Identifier {
	namespace := uuid.UUID(base)
	res := uuid.NewSHA1(namespace, []byte(strconv.FormatInt(i, 10)))
	return identifier.UUID(res)
}

// ChangeUnmarshalJSON unmarshals a Change from JSON bytes.
func ChangeUnmarshalJSON(data []byte) (Change, errors.E) { //nolint:ireturn
	var t struct {
		Type string `json:"type"`
	}
	errE := x.Unmarshal(data, &t)
	if errE != nil {
		return nil, errE
	}
	switch t.Type {
	case "add":
		return changeUnmarshalJSON[AddClaimChange](data)
	case "set":
		return changeUnmarshalJSON[SetClaimChange](data)
	case "remove":
		return changeUnmarshalJSON[RemoveClaimChange](data)
	default:
		return nil, errors.Errorf(`change of type "%s" is not supported`, t.Type)
	}
}

// ChangeMarshalJSON marshals a Change to JSON bytes.
func ChangeMarshalJSON(change Change) ([]byte, errors.E) {
	switch change.(type) {
	case AddClaimChange, SetClaimChange, RemoveClaimChange:
	default:
		return nil, errors.Errorf(`change of type %T is not supported`, change)
	}
	return x.MarshalWithoutEscapeHTML(change)
}

// Changes is a slice of Change operations to apply to a document.
//
//nolint:recvcheck
type Changes []Change

// Apply applies all changes in order to the given document.
func (c Changes) Apply(doc *D) errors.E {
	for i, change := range c {
		errE := change.Apply(doc)
		if errE != nil {
			errors.Details(errE)["change"] = i
			return errE
		}
	}
	return nil
}

// Validate validates all changes in the slice.
func (c Changes) Validate(ctx context.Context, base identifier.Identifier) errors.E {
	for i, change := range c {
		errE := change.Validate(ctx, base, int64(i))
		if errE != nil {
			errors.Details(errE)["change"] = i
			return errE
		}
	}
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for Changes.
func (c *Changes) UnmarshalJSON(data []byte) error {
	var changes []json.RawMessage
	errE := x.UnmarshalWithoutUnknownFields(data, &changes)
	if errE != nil {
		return errE
	}
	*c = nil
	for _, changeJSON := range changes {
		change, errE := ChangeUnmarshalJSON(changeJSON)
		if errE != nil {
			return errE
		}
		*c = append(*c, change)
	}
	return nil
}

// MarshalJSON implements json.Marshaler for Changes.
func (c Changes) MarshalJSON() ([]byte, error) {
	buffer := bytes.Buffer{}
	buffer.WriteString("[")
	// We manually iterate over the slice to make sure only supported changes are in the slice.
	for i, change := range c {
		if i != 0 {
			buffer.WriteString(",")
		}
		data, errE := ChangeMarshalJSON(change)
		if errE != nil {
			return nil, errE
		}
		buffer.Write(data)
	}
	buffer.WriteString("]")
	return buffer.Bytes(), nil
}

func changeUnmarshalJSON[T Change](data []byte) (Change, errors.E) { //nolint:ireturn
	var d T
	errE := x.UnmarshalWithoutUnknownFields(data, &d)
	if errE != nil {
		return nil, errE
	}
	return d, nil
}

func claimPatchUnmarshalJSON[T ClaimPatch](data []byte) (ClaimPatch, errors.E) { //nolint:ireturn
	var d T
	errE := x.UnmarshalWithoutUnknownFields(data, &d)
	if errE != nil {
		return nil, errE
	}
	return d, nil
}

// ClaimPatchUnmarshalJSON unmarshals a ClaimPatch from JSON bytes.
func ClaimPatchUnmarshalJSON(data json.RawMessage) (ClaimPatch, errors.E) { //nolint:ireturn
	var t struct {
		Type string `json:"type"`
	}
	errE := x.Unmarshal(data, &t)
	if errE != nil {
		return nil, errE
	}
	switch t.Type {
	case "id":
		return claimPatchUnmarshalJSON[IdentifierClaimPatch](data)
	case "ref":
		return claimPatchUnmarshalJSON[ReferenceClaimPatch](data)
	case "text":
		return claimPatchUnmarshalJSON[TextClaimPatch](data)
	case "string":
		return claimPatchUnmarshalJSON[StringClaimPatch](data)
	case "amount":
		return claimPatchUnmarshalJSON[AmountClaimPatch](data)
	case "amountRange":
		return claimPatchUnmarshalJSON[AmountRangeClaimPatch](data)
	case "rel":
		return claimPatchUnmarshalJSON[RelationClaimPatch](data)
	case "file":
		return claimPatchUnmarshalJSON[FileClaimPatch](data)
	case "none":
		return claimPatchUnmarshalJSON[NoValueClaimPatch](data)
	case "unknown":
		return claimPatchUnmarshalJSON[UnknownValueClaimPatch](data)
	case "time":
		return claimPatchUnmarshalJSON[TimeClaimPatch](data)
	case "timeRange":
		return claimPatchUnmarshalJSON[TimeRangeClaimPatch](data)
	default:
		return nil, errors.Errorf(`patch of type "%s" is not supported`, t.Type)
	}
}

// ClaimPatchMarshalJSON marshals a ClaimPatch to JSON bytes.
func ClaimPatchMarshalJSON(patch ClaimPatch) ([]byte, errors.E) {
	switch patch.(type) {
	case IdentifierClaimPatch, ReferenceClaimPatch, TextClaimPatch, StringClaimPatch, AmountClaimPatch, AmountRangeClaimPatch,
		RelationClaimPatch, FileClaimPatch, NoValueClaimPatch, UnknownValueClaimPatch, TimeClaimPatch, TimeRangeClaimPatch:
	default:
		return nil, errors.Errorf(`patch of type %T is not supported`, patch)
	}
	return x.MarshalWithoutEscapeHTML(patch)
}

// Change represents a modification operation that can be applied to a document.
type Change interface {
	Apply(doc *D) errors.E
	Validate(ctx context.Context, base identifier.Identifier, operation int64) errors.E
}

var (
	_ Change = AddClaimChange{}
	_ Change = SetClaimChange{}
	_ Change = RemoveClaimChange{}
)

// ClaimPatch represents a modification that can be applied to create or update a claim.
type ClaimPatch interface {
	New(id identifier.Identifier) (Claim, errors.E)
	Apply(claim Claim) errors.E
}

var (
	_ ClaimPatch = IdentifierClaimPatch{}
	_ ClaimPatch = ReferenceClaimPatch{}
	_ ClaimPatch = TextClaimPatch{}
	_ ClaimPatch = StringClaimPatch{}
	_ ClaimPatch = AmountClaimPatch{}
	_ ClaimPatch = AmountRangeClaimPatch{}
	_ ClaimPatch = RelationClaimPatch{}
	_ ClaimPatch = FileClaimPatch{}
	_ ClaimPatch = NoValueClaimPatch{}
	_ ClaimPatch = UnknownValueClaimPatch{}
	_ ClaimPatch = TimeClaimPatch{}
	_ ClaimPatch = TimeRangeClaimPatch{}
)

// AddClaimChange represents a change that adds a new claim to a document.
//
//nolint:recvcheck
type AddClaimChange struct {
	Under *identifier.Identifier `json:"under,omitempty"`
	ID    identifier.Identifier  `json:"id"`
	Patch ClaimPatch             `json:"patch"`
}

// Apply applies the add claim change to the document.
func (c AddClaimChange) Apply(doc *D) errors.E {
	newClaim, errE := c.Patch.New(c.ID)
	if errE != nil {
		return errE
	}

	if c.Under == nil {
		return doc.Add(newClaim)
	}

	claim := doc.GetByID(*c.Under)
	if claim == nil {
		return errors.Errorf(`claim with ID "%s" not found`, *c.Under)
	}
	return claim.Add(newClaim)
}

// Validate validates the add claim change.
func (c AddClaimChange) Validate(_ context.Context, base identifier.Identifier, operation int64) errors.E {
	expectedID := idAtIndex(base, operation)
	if expectedID != c.ID {
		errE := errors.New("invalid ID")
		errors.Details(errE)["id"] = c.ID.String()
		errors.Details(errE)["expected"] = expectedID.String()
		return errE
	}
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for AddClaimChange.
func (c *AddClaimChange) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type C AddClaimChange
	var t struct {
		C

		Type  string          `json:"type"`
		Patch json.RawMessage `json:"patch"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "add" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	patch, errE := ClaimPatchUnmarshalJSON(t.Patch)
	if errE != nil {
		return errE
	}
	c.ID = t.ID
	c.Under = t.Under
	c.Patch = patch
	return nil
}

// MarshalJSON implements json.Marshaler for AddClaimChange.
func (c AddClaimChange) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type C AddClaimChange
	t := struct {
		C

		Type string `json:"type"`
	}{
		C: C(c),

		Type: "add",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// SetClaimChange represents a change that modifies an existing claim in a document.
//
//nolint:recvcheck
type SetClaimChange struct {
	ID    identifier.Identifier `json:"id"`
	Patch ClaimPatch            `json:"patch"`
}

// Apply applies the set claim change to the document.
func (c SetClaimChange) Apply(doc *D) errors.E {
	claim := doc.GetByID(c.ID)
	if claim == nil {
		return errors.Errorf(`claim with ID "%s" not found`, c.ID)
	}
	return c.Patch.Apply(claim)
}

// Validate validates the set claim change.
func (c SetClaimChange) Validate(_ context.Context, _ identifier.Identifier, _ int64) errors.E {
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for SetClaimChange.
func (c *SetClaimChange) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type C SetClaimChange
	var t struct {
		C

		Type  string          `json:"type"`
		Patch json.RawMessage `json:"patch"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "set" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	patch, errE := ClaimPatchUnmarshalJSON(t.Patch)
	if errE != nil {
		return errE
	}
	c.ID = t.ID
	c.Patch = patch
	return nil
}

// MarshalJSON implements json.Marshaler for SetClaimChange.
func (c SetClaimChange) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type C SetClaimChange
	t := struct {
		C

		Type string `json:"type"`
	}{
		C: C(c),

		Type: "set",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// RemoveClaimChange represents a change that removes a claim from a document.
//
//nolint:recvcheck
type RemoveClaimChange struct {
	ID identifier.Identifier `json:"id"`
}

// Apply applies the remove claim change to the document.
func (c RemoveClaimChange) Apply(doc *D) errors.E {
	claim := doc.RemoveByID(c.ID)
	if claim == nil {
		return errors.Errorf(`claim with ID "%s" not found`, c.ID)
	}
	return nil
}

// Validate validates the remove claim change.
func (c RemoveClaimChange) Validate(_ context.Context, _ identifier.Identifier, _ int64) errors.E {
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for RemoveClaimChange.
func (c *RemoveClaimChange) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type C RemoveClaimChange
	var t struct {
		C

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "remove" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	c.ID = t.ID
	return nil
}

// MarshalJSON implements json.Marshaler for RemoveClaimChange.
func (c RemoveClaimChange) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type C RemoveClaimChange
	t := struct {
		C

		Type string `json:"type"`
	}{
		C: C(c),

		Type: "remove",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// IdentifierClaimPatch represents a patch for an identifier claim.
//
//nolint:recvcheck
type IdentifierClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Value      *string                `exhaustruct:"optional" json:"value,omitempty"`
}

// New creates a new identifier claim from the patch.
func (p IdentifierClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.Value == nil {
		return nil, errors.New("incomplete patch")
	}

	return &IdentifierClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		Value: *p.Value,
	}, nil
}

// Apply applies the patch to an existing identifier claim.
func (p IdentifierClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.Value == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*IdentifierClaim)
	if !ok {
		return errors.New("not identifier claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.Value != nil {
		c.Value = *p.Value
	}

	return nil
}

// UnmarshalJSON unmarshals an identifier claim patch from JSON.
func (p *IdentifierClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P IdentifierClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "id" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = IdentifierClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals an identifier claim patch to JSON.
func (p IdentifierClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P IdentifierClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "id",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// ReferenceClaimPatch represents a patch for a reference claim.
//
//nolint:recvcheck
type ReferenceClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	IRI        *string                `exhaustruct:"optional" json:"iri,omitempty"`
}

// New creates a new reference claim from the patch.
func (p ReferenceClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.IRI == nil {
		return nil, errors.New("incomplete patch")
	}

	return &ReferenceClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		IRI: *p.IRI,
	}, nil
}

// Apply applies the patch to an existing reference claim.
func (p ReferenceClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && p.IRI == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*ReferenceClaim)
	if !ok {
		return errors.New("not reference claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.IRI != nil {
		c.IRI = *p.IRI
	}

	return nil
}

// UnmarshalJSON unmarshals a reference claim patch from JSON.
func (p *ReferenceClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P ReferenceClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "ref" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = ReferenceClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a reference claim patch to JSON.
func (p ReferenceClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P ReferenceClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "ref",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// TextClaimPatch represents a patch for a text claim.
//
//nolint:recvcheck
type TextClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	HTML       TranslatableHTMLString `exhaustruct:"optional" json:"html,omitempty"`
	Remove     []string               `exhaustruct:"optional" json:"remove,omitempty"`
}

// New creates a new text claim from the patch.
func (p TextClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || len(p.HTML) == 0 {
		return nil, errors.New("incomplete patch")
	}
	if len(p.Remove) != 0 {
		return nil, errors.New("invalid patch")
	}

	return &TextClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		HTML: p.HTML,
	}, nil
}

// Apply applies the patch to an existing text claim.
func (p TextClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && len(p.HTML) == 0 && len(p.Remove) == 0 {
		return errors.New("empty patch")
	}

	c, ok := claim.(*TextClaim)
	if !ok {
		return errors.New("not text claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	for _, lang := range p.Remove {
		delete(c.HTML, lang)
	}
	for lang, value := range p.HTML {
		c.HTML[lang] = value
	}

	return nil
}

// UnmarshalJSON unmarshals a text claim patch from JSON.
func (p *TextClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TextClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "text" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = TextClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a text claim patch to JSON.
func (p TextClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TextClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "text",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// StringClaimPatch represents a patch for a string claim.
//
//nolint:recvcheck
type StringClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	String     *string                `exhaustruct:"optional" json:"string,omitempty"`
}

// New creates a new string claim from the patch.
func (p StringClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.String == nil {
		return nil, errors.New("incomplete patch")
	}

	return &StringClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		String: *p.String,
	}, nil
}

// Apply applies the patch to an existing string claim.
func (p StringClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.String == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*StringClaim)
	if !ok {
		return errors.New("not string claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.String != nil {
		c.String = *p.String
	}

	return nil
}

// UnmarshalJSON unmarshals a string claim patch from JSON.
func (p *StringClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P StringClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "string" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = StringClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a string claim patch to JSON.
func (p StringClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P StringClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "string",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// AmountClaimPatch represents a patch for an amount claim.
//
//nolint:recvcheck
type AmountClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Amount     *float64               `exhaustruct:"optional" json:"amount,omitempty"`
	Unit       *AmountUnit            `exhaustruct:"optional" json:"unit,omitempty"`
}

// New creates a new amount claim from the patch.
func (p AmountClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.Amount == nil || p.Unit == nil {
		return nil, errors.New("incomplete patch")
	}

	return &AmountClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		Amount: *p.Amount,
		Unit:   *p.Unit,
	}, nil
}

// Apply applies the patch to an existing amount claim.
func (p AmountClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.Amount == nil && p.Unit == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*AmountClaim)
	if !ok {
		return errors.New("not amount claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.Amount != nil {
		c.Amount = *p.Amount
	}
	if p.Unit != nil {
		c.Unit = *p.Unit
	}

	return nil
}

// UnmarshalJSON unmarshals an amount claim patch from JSON.
func (p *AmountClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AmountClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "amount" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = AmountClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals an amount claim patch to JSON.
func (p AmountClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AmountClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "amount",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// AmountRangeClaimPatch represents a patch for an amount range claim.
//
//nolint:recvcheck
type AmountRangeClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Lower      *float64               `exhaustruct:"optional" json:"lower,omitempty"`
	Upper      *float64               `exhaustruct:"optional" json:"upper,omitempty"`
	Unit       *AmountUnit            `exhaustruct:"optional" json:"unit,omitempty"`
}

// New creates a new amount range claim from the patch.
func (p AmountRangeClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.Lower == nil || p.Upper == nil || p.Unit == nil {
		return nil, errors.New("incomplete patch")
	}

	return &AmountRangeClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		Lower: *p.Lower,
		Upper: *p.Upper,
		Unit:  *p.Unit,
	}, nil
}

// Apply applies the patch to an existing amount range claim.
func (p AmountRangeClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.Lower == nil && p.Upper == nil && p.Unit == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*AmountRangeClaim)
	if !ok {
		return errors.New("not amount range claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.Lower != nil {
		c.Lower = *p.Lower
	}
	if p.Upper != nil {
		c.Upper = *p.Upper
	}
	if p.Unit != nil {
		c.Unit = *p.Unit
	}

	return nil
}

// UnmarshalJSON unmarshals an amount range claim patch from JSON.
func (p *AmountRangeClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AmountRangeClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "amountRange" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = AmountRangeClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals an amount range claim patch to JSON.
func (p AmountRangeClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AmountRangeClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "amountRange",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// RelationClaimPatch represents a patch for a relation claim.
//
//nolint:recvcheck
type RelationClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	To         *identifier.Identifier `exhaustruct:"optional" json:"to,omitempty"`
}

// New creates a new relation claim from the patch.
func (p RelationClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.To == nil {
		return nil, errors.New("incomplete patch")
	}

	return &RelationClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		To: Reference{
			ID: p.To,
		},
	}, nil
}

// Apply applies the patch to an existing relation claim.
func (p RelationClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.To == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*RelationClaim)
	if !ok {
		return errors.New("not relation claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.To != nil {
		c.To.ID = p.To
	}

	return nil
}

// UnmarshalJSON unmarshals a relation claim patch from JSON.
func (p *RelationClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P RelationClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "rel" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = RelationClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a relation claim patch to JSON.
func (p RelationClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P RelationClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "rel",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// FileClaimPatch represents a patch for a file claim.
//
//nolint:recvcheck
type FileClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	MediaType  *string                `exhaustruct:"optional" json:"mediaType,omitempty"`
	URL        *string                `exhaustruct:"optional" json:"url,omitempty"`
	Preview    []string               `exhaustruct:"optional" json:"preview"`
}

// New creates a new file claim from the patch.
func (p FileClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.MediaType == nil || p.URL == nil || p.Preview == nil {
		return nil, errors.New("incomplete patch")
	}

	return &FileClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		MediaType: *p.MediaType,
		URL:       *p.URL,
		Preview:   p.Preview,
	}, nil
}

// Apply applies the patch to an existing file claim.
func (p FileClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.MediaType == nil && p.URL == nil && p.Preview == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*FileClaim)
	if !ok {
		return errors.New("not file claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.MediaType != nil {
		c.MediaType = *p.MediaType
	}
	if p.URL != nil {
		c.URL = *p.URL
	}
	if p.Preview != nil {
		c.Preview = p.Preview
	}

	return nil
}

// UnmarshalJSON unmarshals a file claim patch from JSON.
func (p *FileClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P FileClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "file" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = FileClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a file claim patch to JSON.
func (p FileClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P FileClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "file",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// NoValueClaimPatch represents a patch for a no value claim.
//
//nolint:recvcheck
type NoValueClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
}

// New creates a new no value claim from the patch.
func (p NoValueClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil {
		return nil, errors.New("incomplete patch")
	}

	return &NoValueClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
	}, nil
}

// Apply applies the patch to an existing no value claim.
func (p NoValueClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*NoValueClaim)
	if !ok {
		return errors.New("not no value claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}

	return nil
}

// UnmarshalJSON unmarshals a no value claim patch from JSON.
func (p *NoValueClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P NoValueClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "none" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = NoValueClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a no value claim patch to JSON.
func (p NoValueClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P NoValueClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "none",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// UnknownValueClaimPatch represents a patch for an unknown value claim.
//
//nolint:recvcheck
type UnknownValueClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
}

// New creates a new unknown value claim from the patch.
func (p UnknownValueClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil {
		return nil, errors.New("incomplete patch")
	}

	return &UnknownValueClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
	}, nil
}

// Apply applies the patch to an existing unknown value claim.
func (p UnknownValueClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*UnknownValueClaim)
	if !ok {
		return errors.New("not unknown value claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}

	return nil
}

// UnmarshalJSON unmarshals an unknown value claim patch from JSON.
func (p *UnknownValueClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P UnknownValueClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "unknown" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = UnknownValueClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals an unknown value claim patch to JSON.
func (p UnknownValueClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P UnknownValueClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "unknown",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// TimeClaimPatch represents a patch for a time claim.
//
//nolint:recvcheck
type TimeClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Timestamp  *Timestamp             `exhaustruct:"optional" json:"timestamp,omitempty"`
	Precision  *TimePrecision         `exhaustruct:"optional" json:"precision,omitempty"`
}

// New creates a new time claim from the patch.
func (p TimeClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.Timestamp == nil || p.Precision == nil {
		return nil, errors.New("incomplete patch")
	}

	return &TimeClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		Timestamp: *p.Timestamp,
		Precision: *p.Precision,
	}, nil
}

// Apply applies the patch to an existing time claim.
func (p TimeClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.Timestamp == nil && p.Precision == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*TimeClaim)
	if !ok {
		return errors.New("not time claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.Timestamp != nil {
		c.Timestamp = *p.Timestamp
	}
	if p.Precision != nil {
		c.Precision = *p.Precision
	}

	return nil
}

// UnmarshalJSON unmarshals a time claim patch from JSON.
func (p *TimeClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TimeClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "time" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = TimeClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a time claim patch to JSON.
func (p TimeClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TimeClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "time",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// TimeRangeClaimPatch represents a patch for a time range claim.
//
//nolint:recvcheck
type TimeRangeClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Lower      *Timestamp             `exhaustruct:"optional" json:"lower,omitempty"`
	Upper      *Timestamp             `exhaustruct:"optional" json:"upper,omitempty"`
	Precision  *TimePrecision         `exhaustruct:"optional" json:"precision,omitempty"`
}

// New creates a new time range claim from the patch.
func (p TimeRangeClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.Lower == nil || p.Upper == nil || p.Precision == nil {
		return nil, errors.New("incomplete patch")
	}

	return &TimeRangeClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: p.Prop,
		},
		Lower:     *p.Lower,
		Upper:     *p.Upper,
		Precision: *p.Precision,
	}, nil
}

// Apply applies the patch to an existing time range claim.
func (p TimeRangeClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.Lower == nil && p.Upper == nil && p.Precision == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*TimeRangeClaim)
	if !ok {
		return errors.New("not time range claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.Lower != nil {
		c.Lower = *p.Lower
	}
	if p.Upper != nil {
		c.Upper = *p.Upper
	}
	if p.Precision != nil {
		c.Precision = *p.Precision
	}

	return nil
}

// UnmarshalJSON unmarshals a time range claim patch from JSON.
func (p *TimeRangeClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TimeRangeClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "timeRange" {
		return errors.Errorf(`invalid type "%s"`, t.Type)
	}
	*p = TimeRangeClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a time range claim patch to JSON.
func (p TimeRangeClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TimeRangeClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "timeRange",
	}
	return x.MarshalWithoutEscapeHTML(t)
}
