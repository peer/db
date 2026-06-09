package document

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

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
		errE := errors.New("change type not supported")
		errors.Details(errE)["type"] = t.Type
		return nil, errE
	}
}

// ChangeMarshalJSON marshals a Change to JSON bytes.
func ChangeMarshalJSON(change Change) ([]byte, errors.E) {
	switch change.(type) {
	case AddClaimChange, SetClaimChange, RemoveClaimChange:
	default:
		errE := errors.New("change type not supported")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", change)
		return nil, errE
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
func (c Changes) Validate(base []string) errors.E {
	for i, change := range c {
		errE := change.Validate(base, int64(i+1))
		if errE != nil {
			errors.Details(errE)["change"] = i + 1
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
	case "string":
		return claimPatchUnmarshalJSON[StringClaimPatch](data)
	case "html":
		return claimPatchUnmarshalJSON[HTMLClaimPatch](data)
	case "amount":
		return claimPatchUnmarshalJSON[AmountClaimPatch](data)
	case "amountInterval":
		return claimPatchUnmarshalJSON[AmountIntervalClaimPatch](data)
	case "time":
		return claimPatchUnmarshalJSON[TimeClaimPatch](data)
	case "timeInterval":
		return claimPatchUnmarshalJSON[TimeIntervalClaimPatch](data)
	case "link":
		return claimPatchUnmarshalJSON[LinkClaimPatch](data)
	case "ref":
		return claimPatchUnmarshalJSON[ReferenceClaimPatch](data)
	case "has":
		return claimPatchUnmarshalJSON[HasClaimPatch](data)
	case "none":
		return claimPatchUnmarshalJSON[NoneClaimPatch](data)
	case "unknown":
		return claimPatchUnmarshalJSON[UnknownClaimPatch](data)
	default:
		errE := errors.New("patch type not supported")
		errors.Details(errE)["type"] = t.Type
		return nil, errE
	}
}

// ClaimPatchMarshalJSON marshals a ClaimPatch to JSON bytes.
func ClaimPatchMarshalJSON(patch ClaimPatch) ([]byte, errors.E) {
	switch patch.(type) {
	case IdentifierClaimPatch, StringClaimPatch, HTMLClaimPatch, LinkClaimPatch, AmountClaimPatch, AmountIntervalClaimPatch,
		TimeClaimPatch, TimeIntervalClaimPatch, ReferenceClaimPatch, HasClaimPatch, NoneClaimPatch, UnknownClaimPatch:
	default:
		errE := errors.New("patch type not supported")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", patch)
		return nil, errE
	}
	return x.MarshalWithoutEscapeHTML(patch)
}

// Change represents a modification operation that can be applied to a document.
type Change interface {
	Apply(doc *D) errors.E
	Validate(base []string, operation int64) errors.E
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
	_ ClaimPatch = StringClaimPatch{}
	_ ClaimPatch = HTMLClaimPatch{}
	_ ClaimPatch = AmountClaimPatch{}
	_ ClaimPatch = AmountIntervalClaimPatch{}
	_ ClaimPatch = TimeClaimPatch{}
	_ ClaimPatch = TimeIntervalClaimPatch{}
	_ ClaimPatch = LinkClaimPatch{}
	_ ClaimPatch = ReferenceClaimPatch{}
	_ ClaimPatch = HasClaimPatch{}
	_ ClaimPatch = NoneClaimPatch{}
	_ ClaimPatch = UnknownClaimPatch{}
)

// AddClaimChange represents a change that adds a new claim to a document.
//
//nolint:recvcheck
type AddClaimChange struct {
	Under *identifier.Identifier `json:"under,omitempty"`
	ID    identifier.Identifier  `json:"id"`
	Base  []string               `json:"base"`
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
		errE := errors.New("claim not found")
		errors.Details(errE)["id"] = *c.Under
		return errE
	}
	return claim.Add(newClaim)
}

// Validate validates the add claim change.
func (c AddClaimChange) Validate(base []string, operation int64) errors.E {
	expectedBase := slices.Clone(base)
	expectedBase = append(expectedBase, strconv.FormatInt(operation, 10))
	if !slices.Equal(c.Base, expectedBase) {
		errE := errors.New("invalid base")
		errors.Details(errE)["base"] = c.Base
		errors.Details(errE)["expected"] = expectedBase
		return errE
	}
	expectedID := identifier.From(c.Base...)
	if c.ID != expectedID {
		errE := errors.New("invalid ID")
		errors.Details(errE)["id"] = c.ID.String()
		errors.Details(errE)["expected"] = expectedID.String()
		return errE
	}
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for AddClaimChange.
func (c *AddClaimChange) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
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
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
	}
	patch, errE := ClaimPatchUnmarshalJSON(t.Patch)
	if errE != nil {
		return errE
	}
	c.ID = t.ID
	c.Base = t.Base
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
		errE := errors.New("claim not found")
		errors.Details(errE)["id"] = c.ID
		return errE
	}
	return c.Patch.Apply(claim)
}

// Validate validates the set claim change.
func (c SetClaimChange) Validate(_ []string, _ int64) errors.E {
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for SetClaimChange.
func (c *SetClaimChange) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
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
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
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
		errE := errors.New("claim not found")
		errors.Details(errE)["id"] = c.ID
		return errE
	}
	return nil
}

// Validate validates the remove claim change.
func (c RemoveClaimChange) Validate(_ []string, _ int64) errors.E {
	return nil
}

// UnmarshalJSON implements json.Unmarshaler for RemoveClaimChange.
func (c *RemoveClaimChange) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
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
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
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
	Value      string                 `exhaustruct:"optional" json:"value,omitempty"`
}

// New creates a new identifier claim from the patch.
func (p IdentifierClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || len(p.Value) == 0 {
		return nil, errors.New("incomplete patch")
	}

	c := &IdentifierClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
		Value: p.Value,
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing identifier claim.
func (p IdentifierClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && len(p.Value) == 0 {
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
		c.Prop.ID = *p.Prop
	}
	if len(p.Value) > 0 {
		c.Value = p.Value
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals an identifier claim patch from JSON.
func (p *IdentifierClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
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
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
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

// StringClaimPatch represents a patch for a string claim.
//
//nolint:recvcheck
type StringClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	String     string                 `exhaustruct:"optional" json:"string,omitempty"`
}

// New creates a new string claim from the patch.
func (p StringClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || len(p.String) == 0 {
		return nil, errors.New("incomplete patch")
	}

	c := &StringClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
		String: p.String,
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing string claim.
func (p StringClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && len(p.String) == 0 {
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
		c.Prop.ID = *p.Prop
	}
	if len(p.String) > 0 {
		c.String = p.String
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals a string claim patch from JSON.
func (p *StringClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
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
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
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

// HTMLClaimPatch represents a patch for an HTML claim.
//
//nolint:recvcheck
type HTMLClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	HTML       string                 `exhaustruct:"optional" json:"html,omitempty"`
}

// New creates a new HTML claim from the patch.
func (p HTMLClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || len(p.HTML) == 0 {
		return nil, errors.New("incomplete patch")
	}

	c := &HTMLClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
		HTML: p.HTML,
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing HTML claim.
func (p HTMLClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && len(p.HTML) == 0 {
		return errors.New("empty patch")
	}

	c, ok := claim.(*HTMLClaim)
	if !ok {
		return errors.New("not HTML claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = *p.Prop
	}
	if len(p.HTML) > 0 {
		c.HTML = p.HTML
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals an HTML claim patch from JSON.
func (p *HTMLClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
	type P HTMLClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "html" {
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
	}
	*p = HTMLClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals an HTML claim patch to JSON.
func (p HTMLClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P HTMLClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "html",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// AmountClaimPatch represents a patch for an amount claim.
//
//nolint:recvcheck
type AmountClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Amount     *Amount                `exhaustruct:"optional" json:"amount,omitempty"`
	Precision  *float64               `exhaustruct:"optional" json:"precision,omitempty"`
}

// New creates a new amount claim from the patch.
func (p AmountClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.Amount == nil || p.Precision == nil {
		return nil, errors.New("incomplete patch")
	}

	c := &AmountClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
		Amount:    *p.Amount,
		Precision: *p.Precision,
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing amount claim.
func (p AmountClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.Amount == nil && p.Precision == nil {
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
		c.Prop.ID = *p.Prop
	}
	if p.Amount != nil {
		c.Amount = *p.Amount
	}
	if p.Precision != nil {
		c.Precision = *p.Precision
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals an amount claim patch from JSON.
func (p *AmountClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
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
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
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

// AmountIntervalClaimPatch represents a patch for an amount interval claim.
//
//nolint:recvcheck
type AmountIntervalClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`

	From          *Amount  `exhaustruct:"optional" json:"from,omitempty"`
	FromPrecision *float64 `exhaustruct:"optional" json:"fromPrecision,omitempty"`
	FromIsOpen    *bool    `exhaustruct:"optional" json:"fromIsOpen,omitempty"`
	FromIsUnknown *bool    `exhaustruct:"optional" json:"fromIsUnknown,omitempty"`
	FromIsNone    *bool    `exhaustruct:"optional" json:"fromIsNone,omitempty"`

	To          *Amount  `exhaustruct:"optional" json:"to,omitempty"`
	ToPrecision *float64 `exhaustruct:"optional" json:"toPrecision,omitempty"`
	ToIsOpen    *bool    `exhaustruct:"optional" json:"toIsOpen,omitempty"`
	ToIsUnknown *bool    `exhaustruct:"optional" json:"toIsUnknown,omitempty"`
	ToIsNone    *bool    `exhaustruct:"optional" json:"toIsNone,omitempty"`
}

// New creates a new amount interval claim from the patch.
func (p AmountIntervalClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:dupl,ireturn
	if p.Confidence == nil || p.Prop == nil {
		return nil, errors.New("incomplete patch")
	}

	c := &AmountIntervalClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
		From:          p.From,
		FromPrecision: p.FromPrecision,
		FromIsOpen:    p.FromIsOpen != nil && *p.FromIsOpen,
		FromIsUnknown: p.FromIsUnknown != nil && *p.FromIsUnknown,
		FromIsNone:    p.FromIsNone != nil && *p.FromIsNone,
		To:            p.To,
		ToPrecision:   p.ToPrecision,
		ToIsOpen:      p.ToIsOpen != nil && *p.ToIsOpen,
		ToIsUnknown:   p.ToIsUnknown != nil && *p.ToIsUnknown,
		ToIsNone:      p.ToIsNone != nil && *p.ToIsNone,
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing amount interval claim.
func (p AmountIntervalClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil &&
		p.From == nil && p.FromPrecision == nil && p.FromIsOpen == nil && p.FromIsUnknown == nil && p.FromIsNone == nil &&
		p.To == nil && p.ToPrecision == nil && p.ToIsOpen == nil && p.ToIsUnknown == nil && p.ToIsNone == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*AmountIntervalClaim)
	if !ok {
		return errors.New("not amount interval claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = *p.Prop
	}
	if p.From != nil {
		c.From = p.From
		c.FromIsUnknown = false
		c.FromIsNone = false
	}
	if p.FromPrecision != nil {
		c.FromPrecision = p.FromPrecision
	}
	if p.FromIsOpen != nil {
		c.FromIsOpen = *p.FromIsOpen
		if *p.FromIsOpen {
			c.FromIsUnknown = false
			c.FromIsNone = false
		}
	}
	if p.FromIsUnknown != nil {
		c.FromIsUnknown = *p.FromIsUnknown
		if *p.FromIsUnknown {
			c.From = nil
			c.FromPrecision = nil
			c.FromIsOpen = false
			c.FromIsNone = false
		}
	}
	if p.FromIsNone != nil {
		c.FromIsNone = *p.FromIsNone
		if *p.FromIsNone {
			c.From = nil
			c.FromPrecision = nil
			c.FromIsOpen = false
			c.FromIsUnknown = false
		}
	}
	if p.To != nil {
		c.To = p.To
		c.ToIsUnknown = false
		c.ToIsNone = false
	}
	if p.ToPrecision != nil {
		c.ToPrecision = p.ToPrecision
	}
	if p.ToIsOpen != nil {
		c.ToIsOpen = *p.ToIsOpen
		if *p.ToIsOpen {
			c.ToIsUnknown = false
			c.ToIsNone = false
		}
	}
	if p.ToIsUnknown != nil {
		c.ToIsUnknown = *p.ToIsUnknown
		if *p.ToIsUnknown {
			c.To = nil
			c.ToPrecision = nil
			c.ToIsOpen = false
			c.ToIsNone = false
		}
	}
	if p.ToIsNone != nil {
		c.ToIsNone = *p.ToIsNone
		if *p.ToIsNone {
			c.To = nil
			c.ToPrecision = nil
			c.ToIsOpen = false
			c.ToIsUnknown = false
		}
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals an amount interval claim patch from JSON.
func (p *AmountIntervalClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
	type P AmountIntervalClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "amountInterval" {
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
	}
	*p = AmountIntervalClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals an amount interval claim patch to JSON.
func (p AmountIntervalClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AmountIntervalClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "amountInterval",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// TimeClaimPatch represents a patch for a time claim.
//
//nolint:recvcheck
type TimeClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Time       *Time                  `exhaustruct:"optional" json:"time,omitempty"`
	Precision  *TimePrecision         `exhaustruct:"optional" json:"precision,omitempty"`
}

// New creates a new time claim from the patch.
func (p TimeClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.Time == nil || p.Precision == nil {
		return nil, errors.New("incomplete patch")
	}

	c := &TimeClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
		Time:      *p.Time,
		Precision: *p.Precision,
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing time claim.
func (p TimeClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.Time == nil && p.Precision == nil {
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
		c.Prop.ID = *p.Prop
	}
	if p.Time != nil {
		c.Time = *p.Time
	}
	if p.Precision != nil {
		c.Precision = *p.Precision
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals a time claim patch from JSON.
func (p *TimeClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
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
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
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

// TimeIntervalClaimPatch represents a patch for a time interval claim.
//
//nolint:recvcheck
type TimeIntervalClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`

	From          *Time          `exhaustruct:"optional" json:"from,omitempty"`
	FromPrecision *TimePrecision `exhaustruct:"optional" json:"fromPrecision,omitempty"`
	FromIsOpen    *bool          `exhaustruct:"optional" json:"fromIsOpen,omitempty"`
	FromIsUnknown *bool          `exhaustruct:"optional" json:"fromIsUnknown,omitempty"`
	FromIsNone    *bool          `exhaustruct:"optional" json:"fromIsNone,omitempty"`

	To          *Time          `exhaustruct:"optional" json:"to,omitempty"`
	ToPrecision *TimePrecision `exhaustruct:"optional" json:"toPrecision,omitempty"`
	ToIsOpen    *bool          `exhaustruct:"optional" json:"toIsOpen,omitempty"`
	ToIsUnknown *bool          `exhaustruct:"optional" json:"toIsUnknown,omitempty"`
	ToIsNone    *bool          `exhaustruct:"optional" json:"toIsNone,omitempty"`
}

// New creates a new time interval claim from the patch.
func (p TimeIntervalClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:dupl,ireturn
	if p.Confidence == nil || p.Prop == nil {
		return nil, errors.New("incomplete patch")
	}

	c := &TimeIntervalClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
		From:          p.From,
		FromPrecision: p.FromPrecision,
		FromIsOpen:    p.FromIsOpen != nil && *p.FromIsOpen,
		FromIsUnknown: p.FromIsUnknown != nil && *p.FromIsUnknown,
		FromIsNone:    p.FromIsNone != nil && *p.FromIsNone,
		To:            p.To,
		ToPrecision:   p.ToPrecision,
		ToIsOpen:      p.ToIsOpen != nil && *p.ToIsOpen,
		ToIsUnknown:   p.ToIsUnknown != nil && *p.ToIsUnknown,
		ToIsNone:      p.ToIsNone != nil && *p.ToIsNone,
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing time interval claim.
func (p TimeIntervalClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil &&
		p.From == nil && p.FromPrecision == nil && p.FromIsOpen == nil && p.FromIsUnknown == nil && p.FromIsNone == nil &&
		p.To == nil && p.ToPrecision == nil && p.ToIsOpen == nil && p.ToIsUnknown == nil && p.ToIsNone == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*TimeIntervalClaim)
	if !ok {
		return errors.New("not time interval claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = *p.Prop
	}
	if p.From != nil {
		c.From = p.From
		c.FromIsUnknown = false
		c.FromIsNone = false
	}
	if p.FromPrecision != nil {
		c.FromPrecision = p.FromPrecision
	}
	if p.FromIsOpen != nil {
		c.FromIsOpen = *p.FromIsOpen
		if *p.FromIsOpen {
			c.FromIsUnknown = false
			c.FromIsNone = false
		}
	}
	if p.FromIsUnknown != nil {
		c.FromIsUnknown = *p.FromIsUnknown
		if *p.FromIsUnknown {
			c.From = nil
			c.FromPrecision = nil
			c.FromIsOpen = false
			c.FromIsNone = false
		}
	}
	if p.FromIsNone != nil {
		c.FromIsNone = *p.FromIsNone
		if *p.FromIsNone {
			c.From = nil
			c.FromPrecision = nil
			c.FromIsOpen = false
			c.FromIsUnknown = false
		}
	}
	if p.To != nil {
		c.To = p.To
		c.ToIsUnknown = false
		c.ToIsNone = false
	}
	if p.ToPrecision != nil {
		c.ToPrecision = p.ToPrecision
	}
	if p.ToIsOpen != nil {
		c.ToIsOpen = *p.ToIsOpen
		if *p.ToIsOpen {
			c.ToIsUnknown = false
			c.ToIsNone = false
		}
	}
	if p.ToIsUnknown != nil {
		c.ToIsUnknown = *p.ToIsUnknown
		if *p.ToIsUnknown {
			c.To = nil
			c.ToPrecision = nil
			c.ToIsOpen = false
			c.ToIsNone = false
		}
	}
	if p.ToIsNone != nil {
		c.ToIsNone = *p.ToIsNone
		if *p.ToIsNone {
			c.To = nil
			c.ToPrecision = nil
			c.ToIsOpen = false
			c.ToIsUnknown = false
		}
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals a time interval claim patch from JSON.
func (p *TimeIntervalClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
	type P TimeIntervalClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "timeInterval" {
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
	}
	*p = TimeIntervalClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a time interval claim patch to JSON.
func (p TimeIntervalClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TimeIntervalClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "timeInterval",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// LinkClaimPatch represents a patch for a link claim.
//
//nolint:recvcheck
type LinkClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	IRI        string                 `exhaustruct:"optional" json:"iri,omitempty"`
}

// New creates a new link claim from the patch.
func (p LinkClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || len(p.IRI) == 0 {
		return nil, errors.New("incomplete patch")
	}

	c := &LinkClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
		IRI: p.IRI,
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing link claim.
func (p LinkClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && len(p.IRI) == 0 {
		return errors.New("empty patch")
	}

	c, ok := claim.(*LinkClaim)
	if !ok {
		return errors.New("not link claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = *p.Prop
	}
	if len(p.IRI) > 0 {
		c.IRI = p.IRI
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals a link claim patch from JSON.
func (p *LinkClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
	type P LinkClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "link" {
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
	}
	*p = LinkClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a link claim patch to JSON.
func (p LinkClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P LinkClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "link",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// ReferenceClaimPatch represents a patch for a reference claim.
//
//nolint:recvcheck
type ReferenceClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	To         *identifier.Identifier `exhaustruct:"optional" json:"to,omitempty"`
}

// New creates a new reference claim from the patch.
func (p ReferenceClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil || p.To == nil {
		return nil, errors.New("incomplete patch")
	}

	c := &ReferenceClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
		To: Reference{
			ID: *p.To,
		},
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing reference claim.
func (p ReferenceClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil && p.To == nil {
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
		c.Prop.ID = *p.Prop
	}
	if p.To != nil {
		c.To.ID = *p.To
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals a reference claim patch from JSON.
func (p *ReferenceClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
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
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
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

// HasClaimPatch represents a patch for a has claim.
//
//nolint:recvcheck
type HasClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
}

// New creates a new has claim from the patch.
func (p HasClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil {
		return nil, errors.New("incomplete patch")
	}

	c := &HasClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing has claim.
func (p HasClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*HasClaim)
	if !ok {
		return errors.New("not has claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = *p.Prop
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals a has claim patch from JSON.
func (p *HasClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
	type P HasClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "has" {
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
	}
	*p = HasClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a has claim patch to JSON.
func (p HasClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P HasClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "has",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// NoneClaimPatch represents a patch for a none claim.
//
//nolint:recvcheck
type NoneClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
}

// New creates a new none claim from the patch.
func (p NoneClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil {
		return nil, errors.New("incomplete patch")
	}

	c := &NoneClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing none claim.
func (p NoneClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*NoneClaim)
	if !ok {
		return errors.New("not none claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = *p.Prop
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals a none claim patch from JSON.
func (p *NoneClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
	type P NoneClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "none" {
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
	}
	*p = NoneClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals a none claim patch to JSON.
func (p NoneClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P NoneClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "none",
	}
	return x.MarshalWithoutEscapeHTML(t)
}

// UnknownClaimPatch represents a patch for an unknown claim.
//
//nolint:recvcheck
type UnknownClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
}

// New creates a new unknown claim from the patch.
func (p UnknownClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Confidence == nil || p.Prop == nil {
		return nil, errors.New("incomplete patch")
	}

	c := &UnknownClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: *p.Confidence,
		},
		Prop: Reference{
			ID: *p.Prop,
		},
	}

	return c, c.Validate()
}

// Apply applies the patch to an existing unknown claim.
func (p UnknownClaimPatch) Apply(claim Claim) errors.E {
	if p.Confidence == nil && p.Prop == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*UnknownClaim)
	if !ok {
		return errors.New("not unknown claim")
	}

	if p.Confidence != nil {
		c.Confidence = *p.Confidence
	}
	if p.Prop != nil {
		c.Prop.ID = *p.Prop
	}

	return c.Validate()
}

// UnmarshalJSON unmarshals an unknown claim patch from JSON.
func (p *UnknownClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same UnmarshalJSON.
	type P UnknownClaimPatch
	var t struct {
		P

		Type string `json:"type"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &t)
	if errE != nil {
		return errE
	}
	if t.Type != "unknown" {
		errE := errors.New("invalid type")
		errors.Details(errE)["type"] = t.Type
		return errE
	}
	*p = UnknownClaimPatch(t.P)
	return nil
}

// MarshalJSON marshals an unknown claim patch to JSON.
func (p UnknownClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P UnknownClaimPatch
	t := struct {
		P

		Type string `json:"type"`
	}{
		P: P(p),

		Type: "unknown",
	}
	return x.MarshalWithoutEscapeHTML(t)
}
