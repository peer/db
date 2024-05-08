package document

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

func isJSONArray(data []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	t, err := dec.Token()
	if err != nil {
		return false
	}
	return t == '['
}

func ChangeUnmarshalJSON(data []byte) (Change, errors.E) { //nolint:ireturn
	if isJSONArray(data) {
		var changes Changes
		errE := x.UnmarshalWithoutUnknownFields(data, &changes)
		if errE != nil {
			return nil, errE
		}
		return changes, nil
	}

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

func ChangeMarshalJSON(change Change) ([]byte, errors.E) {
	switch change.(type) {
	case AddClaimChange, SetClaimChange, RemoveClaimChange, Changes:
	default:
		return nil, errors.Errorf(`change of type %T is not supported`, change)
	}
	return x.MarshalWithoutEscapeHTML(change)
}

type Changes []Change

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

func (c Changes) Apply(doc *D, base identifier.Identifier) errors.E {
	namespace := uuid.UUID(base)
	for i, change := range c {
		res := uuid.NewSHA1(namespace, []byte(strconv.Itoa(i)))
		id := identifier.FromUUID(res)
		errE := change.Apply(doc, id)
		if errE != nil {
			errors.Details(errE)["change"] = i
			return errE
		}
	}
	return nil
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

func ClaimPatchMarshalJSON(patch ClaimPatch) ([]byte, errors.E) {
	switch patch.(type) {
	case IdentifierClaimPatch, ReferenceClaimPatch, TextClaimPatch, StringClaimPatch, AmountClaimPatch, AmountRangeClaimPatch,
		RelationClaimPatch, FileClaimPatch, NoValueClaimPatch, UnknownValueClaimPatch, TimeClaimPatch, TimeRangeClaimPatch:
	default:
		return nil, errors.Errorf(`patch of type %T is not supported`, patch)
	}
	return x.MarshalWithoutEscapeHTML(patch)
}

type Change interface {
	Apply(doc *D, id identifier.Identifier) errors.E
}

var (
	_ Change = AddClaimChange{}    //nolint:exhaustruct
	_ Change = SetClaimChange{}    //nolint:exhaustruct
	_ Change = RemoveClaimChange{} //nolint:exhaustruct
	_ Change = Changes{}
)

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

type AddClaimChange struct {
	Under *identifier.Identifier `json:"under,omitempty"`
	Patch ClaimPatch             `json:"patch"`
}

func (c AddClaimChange) Apply(doc *D, id identifier.Identifier) errors.E {
	newClaim, errE := c.Patch.New(id)
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

func (c *AddClaimChange) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type C AddClaimChange
	var t struct {
		Type  string          `json:"type"`
		Patch json.RawMessage `json:"patch"`
		C
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
	c.Under = t.Under
	c.Patch = patch
	return nil
}

func (c AddClaimChange) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type C AddClaimChange
	t := struct {
		Type string `json:"type"`
		C
	}{
		Type: "add",
		C:    C(c),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type SetClaimChange struct {
	ID    identifier.Identifier `json:"id"`
	Patch ClaimPatch            `json:"patch"`
}

func (c SetClaimChange) Apply(doc *D, _ identifier.Identifier) errors.E {
	claim := doc.GetByID(c.ID)
	if claim == nil {
		return errors.Errorf(`claim with ID "%s" not found`, c.ID)
	}
	return c.Patch.Apply(claim)
}

func (c *SetClaimChange) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type C SetClaimChange
	var t struct {
		Type  string          `json:"type"`
		Patch json.RawMessage `json:"patch"`
		C
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

func (c SetClaimChange) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type C SetClaimChange
	t := struct {
		Type string `json:"type"`
		C
	}{
		Type: "set",
		C:    C(c),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type RemoveClaimChange struct {
	ID identifier.Identifier `json:"id"`
}

func (c RemoveClaimChange) Apply(doc *D, _ identifier.Identifier) errors.E {
	claim := doc.RemoveByID(c.ID)
	if claim == nil {
		return errors.Errorf(`claim with ID "%s" not found`, c.ID)
	}
	return nil
}

func (c *RemoveClaimChange) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type C RemoveClaimChange
	var t struct {
		Type string `json:"type"`
		C
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

func (c RemoveClaimChange) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type C RemoveClaimChange
	t := struct {
		Type string `json:"type"`
		C
	}{
		Type: "remove",
		C:    C(c),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type IdentifierClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Value      *string                `exhaustruct:"optional" json:"value,omitempty"`
}

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

func (p *IdentifierClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P IdentifierClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p IdentifierClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P IdentifierClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "id",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type ReferenceClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	IRI        *string                `exhaustruct:"optional" json:"iri,omitempty"`
}

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

func (p *ReferenceClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P ReferenceClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p ReferenceClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P ReferenceClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "ref",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type TextClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	HTML       TranslatableHTMLString `exhaustruct:"optional" json:"html,omitempty"`
	Remove     []string               `exhaustruct:"optional" json:"remove,omitempty"`
}

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

func (p *TextClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TextClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p TextClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TextClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "text",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type StringClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	String     *string                `exhaustruct:"optional" json:"string,omitempty"`
}

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

func (p *StringClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P StringClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p StringClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P StringClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "string",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type AmountClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Amount     *float64               `exhaustruct:"optional" json:"amount,omitempty"`
	Unit       *AmountUnit            `exhaustruct:"optional" json:"unit,omitempty"`
}

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

func (p *AmountClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AmountClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p AmountClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AmountClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "amount",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type AmountRangeClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Lower      *float64               `exhaustruct:"optional" json:"lower,omitempty"`
	Upper      *float64               `exhaustruct:"optional" json:"upper,omitempty"`
	Unit       *AmountUnit            `exhaustruct:"optional" json:"unit,omitempty"`
}

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

func (p *AmountRangeClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AmountRangeClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p AmountRangeClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AmountRangeClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "amountRange",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type RelationClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	To         *identifier.Identifier `exhaustruct:"optional" json:"to,omitempty"`
}

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

func (p *RelationClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P RelationClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p RelationClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P RelationClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "rel",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type FileClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	MediaType  *string                `exhaustruct:"optional" json:"mediaType,omitempty"`
	URL        *string                `exhaustruct:"optional" json:"url,omitempty"`
	Preview    []string               `exhaustruct:"optional" json:"preview"`
}

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

func (p *FileClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P FileClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p FileClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P FileClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "file",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type NoValueClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
}

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

func (p *NoValueClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P NoValueClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p NoValueClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P NoValueClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "none",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type UnknownValueClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
}

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

func (p *UnknownValueClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P UnknownValueClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p UnknownValueClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P UnknownValueClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "unknown",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type TimeClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Timestamp  *Timestamp             `exhaustruct:"optional" json:"timestamp,omitempty"`
	Precision  *TimePrecision         `exhaustruct:"optional" json:"precision,omitempty"`
}

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

func (p *TimeClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TimeClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p TimeClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TimeClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "time",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}

type TimeRangeClaimPatch struct {
	Confidence *Confidence            `exhaustruct:"optional" json:"confidence,omitempty"`
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Lower      *Timestamp             `exhaustruct:"optional" json:"lower,omitempty"`
	Upper      *Timestamp             `exhaustruct:"optional" json:"upper,omitempty"`
	Precision  *TimePrecision         `exhaustruct:"optional" json:"precision,omitempty"`
}

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

func (p *TimeRangeClaimPatch) UnmarshalJSON(data []byte) error {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TimeRangeClaimPatch
	var t struct {
		Type string `json:"type"`
		P
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

func (p TimeRangeClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P TimeRangeClaimPatch
	t := struct {
		Type string `json:"type"`
		P
	}{
		Type: "timeRange",
		P:    P(p),
	}
	return x.MarshalWithoutEscapeHTML(t)
}
