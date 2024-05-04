package document

import (
	"bytes"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

func addClaimPatchUnmarshalJSON[T ClaimPatch](p *AddClaimPatch, data []byte) errors.E {
	var d struct {
		Type  string                 `json:"type"`
		Under *identifier.Identifier `json:"under,omitempty"`
		Patch T                      `json:"patch"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &d)
	if errE != nil {
		return errE
	}
	p.Under = d.Under
	p.Patch = d.Patch
	return nil
}

func setClaimPatchUnmarshalJSON[T ClaimPatch](p *SetClaimPatch, data []byte) errors.E {
	var d struct {
		Type  string                `json:"type"`
		ID    identifier.Identifier `json:"id"`
		Patch T                     `json:"patch"`
	}
	errE := x.UnmarshalWithoutUnknownFields(data, &d)
	if errE != nil {
		return errE
	}
	p.ID = d.ID
	p.Patch = d.Patch
	return nil
}

func appendPatchType(data []byte, patch ClaimPatch) ([]byte, errors.E) {
	buffer := bytes.NewBuffer(data)

	// We remove trailing }.
	buffer.Truncate(buffer.Len() - 1)
	buffer.WriteString(`,"type":`)

	switch patch.(type) {
	case IdentifierClaimPatch:
		buffer.WriteString(`"id"`)
	case ReferenceClaimPatch:
		buffer.WriteString(`"ref"`)
	case TextClaimPatch:
		buffer.WriteString(`"text"`)
	case StringClaimPatch:
		buffer.WriteString(`"string"`)
	case AmountClaimPatch:
		buffer.WriteString(`"amount"`)
	case AmountRangeClaimPatch:
		buffer.WriteString(`"amountRange"`)
	case RelationClaimPatch:
		buffer.WriteString(`"rel"`)
	case FileClaimPatch:
		buffer.WriteString(`"file"`)
	case NoValueClaimPatch:
		buffer.WriteString(`"none"`)
	case UnknownValueClaimPatch:
		buffer.WriteString(`"unknown"`)
	case TimeClaimPatch:
		buffer.WriteString(`"time"`)
	case TimeRangeClaimPatch:
		buffer.WriteString(`"timeRange"`)
	default:
		return nil, errors.Errorf(`patch of type %T is not supported`, patch)
	}

	buffer.WriteString(`}`)

	return buffer.Bytes(), nil
}

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

type AddClaimPatch struct {
	Under *identifier.Identifier `json:"under,omitempty"`
	Patch ClaimPatch             `json:"patch"`
}

func (p *AddClaimPatch) UnmarshalJSON(data []byte) error { //nolint:dupl
	var t struct {
		Type string `json:"type"`
	}
	errE := x.Unmarshal(data, &t)
	if errE != nil {
		return errE
	}
	switch t.Type {
	case "id":
		return addClaimPatchUnmarshalJSON[IdentifierClaimPatch](p, data)
	case "ref":
		return addClaimPatchUnmarshalJSON[ReferenceClaimPatch](p, data)
	case "text":
		return addClaimPatchUnmarshalJSON[TextClaimPatch](p, data)
	case "string":
		return addClaimPatchUnmarshalJSON[StringClaimPatch](p, data)
	case "amount":
		return addClaimPatchUnmarshalJSON[AmountClaimPatch](p, data)
	case "amountRange":
		return addClaimPatchUnmarshalJSON[AmountRangeClaimPatch](p, data)
	case "rel":
		return addClaimPatchUnmarshalJSON[RelationClaimPatch](p, data)
	case "file":
		return addClaimPatchUnmarshalJSON[FileClaimPatch](p, data)
	case "none":
		return addClaimPatchUnmarshalJSON[NoValueClaimPatch](p, data)
	case "unknown":
		return addClaimPatchUnmarshalJSON[UnknownValueClaimPatch](p, data)
	case "time":
		return addClaimPatchUnmarshalJSON[TimeClaimPatch](p, data)
	case "timeRange":
		return addClaimPatchUnmarshalJSON[TimeRangeClaimPatch](p, data)
	default:
		return errors.Errorf(`patch of type "%s" is not supported`, t.Type)
	}
}

func (p AddClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P AddClaimPatch
	data, errE := x.MarshalWithoutEscapeHTML(P(p))
	if errE != nil {
		return nil, errE
	}
	return appendPatchType(data, p.Patch)
}

func (p AddClaimPatch) Apply(doc *D, id identifier.Identifier) errors.E {
	c, errE := p.Patch.New(id)
	if errE != nil {
		return errE
	}

	if p.Under == nil {
		return doc.Add(c)
	}

	claim := doc.GetByID(*p.Under)
	if claim == nil {
		return errors.Errorf(`claim with ID "%s" not found`, *p.Under)
	}
	return claim.Add(c)
}

type SetClaimPatch struct {
	ID    identifier.Identifier `json:"id"`
	Patch ClaimPatch            `json:"patch"`
}

func (p *SetClaimPatch) UnmarshalJSON(data []byte) error { //nolint:dupl
	var t struct {
		Type string `json:"type"`
	}
	errE := x.Unmarshal(data, &t)
	if errE != nil {
		return errE
	}
	switch t.Type {
	case "id":
		return setClaimPatchUnmarshalJSON[IdentifierClaimPatch](p, data)
	case "ref":
		return setClaimPatchUnmarshalJSON[ReferenceClaimPatch](p, data)
	case "text":
		return setClaimPatchUnmarshalJSON[TextClaimPatch](p, data)
	case "string":
		return setClaimPatchUnmarshalJSON[StringClaimPatch](p, data)
	case "amount":
		return setClaimPatchUnmarshalJSON[AmountClaimPatch](p, data)
	case "amountRange":
		return setClaimPatchUnmarshalJSON[AmountRangeClaimPatch](p, data)
	case "rel":
		return setClaimPatchUnmarshalJSON[RelationClaimPatch](p, data)
	case "file":
		return setClaimPatchUnmarshalJSON[FileClaimPatch](p, data)
	case "none":
		return setClaimPatchUnmarshalJSON[NoValueClaimPatch](p, data)
	case "unknown":
		return setClaimPatchUnmarshalJSON[UnknownValueClaimPatch](p, data)
	case "time":
		return setClaimPatchUnmarshalJSON[TimeClaimPatch](p, data)
	case "timeRange":
		return setClaimPatchUnmarshalJSON[TimeRangeClaimPatch](p, data)
	default:
		return errors.Errorf(`patch of type "%s" is not supported`, t.Type)
	}
}

func (p SetClaimPatch) MarshalJSON() ([]byte, error) {
	// We define a new type to not recurse into this same MarshalJSON.
	type P SetClaimPatch
	data, errE := x.MarshalWithoutEscapeHTML(P(p))
	if errE != nil {
		return nil, errE
	}
	return appendPatchType(data, p.Patch)
}

func (p SetClaimPatch) Apply(doc *D) errors.E {
	claim := doc.GetByID(p.ID)
	if claim == nil {
		return errors.Errorf(`claim with ID "%s" not found`, p.ID)
	}
	return p.Patch.Apply(claim)
}

type RemoveClaimPatch struct {
	ID identifier.Identifier `json:"id"`
}

func (p RemoveClaimPatch) Apply(doc *D) errors.E {
	claim := doc.RemoveByID(p.ID)
	if claim == nil {
		return errors.Errorf(`claim with ID "%s" not found`, p.ID)
	}
	return nil
}

type IdentifierClaimPatch struct {
	Prop       *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Identifier *string                `exhaustruct:"optional" json:"id,omitempty"`
}

func (p IdentifierClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || p.Identifier == nil {
		return nil, errors.New("incomplete patch")
	}

	return &IdentifierClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
		Identifier: *p.Identifier,
	}, nil
}

func (p IdentifierClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && p.Identifier == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*IdentifierClaim)
	if !ok {
		return errors.New("not identifier claim")
	}

	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.Identifier != nil {
		c.Identifier = *p.Identifier
	}

	return nil
}

type ReferenceClaimPatch struct {
	Prop *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	IRI  *string                `exhaustruct:"optional" json:"iri,omitempty"`
}

func (p ReferenceClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || p.IRI == nil {
		return nil, errors.New("incomplete patch")
	}

	return &ReferenceClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
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

	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.IRI != nil {
		c.IRI = *p.IRI
	}

	return nil
}

type TextClaimPatch struct {
	Prop   *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	HTML   TranslatableHTMLString `exhaustruct:"optional" json:"html,omitempty"`
	Remove []string               `exhaustruct:"optional" json:"remove,omitempty"`
}

func (p TextClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || len(p.HTML) == 0 {
		return nil, errors.New("incomplete patch")
	}
	if len(p.Remove) != 0 {
		return nil, errors.New("invalid patch")
	}

	return &TextClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
		HTML: p.HTML,
	}, nil
}

func (p TextClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && len(p.HTML) == 0 && len(p.Remove) == 0 {
		return errors.New("empty patch")
	}

	c, ok := claim.(*TextClaim)
	if !ok {
		return errors.New("not text claim")
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

type StringClaimPatch struct {
	Prop   *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	String *string                `exhaustruct:"optional" json:"string,omitempty"`
}

func (p StringClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || p.String == nil {
		return nil, errors.New("incomplete patch")
	}

	return &StringClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
		String: *p.String,
	}, nil
}

func (p StringClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && p.String == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*StringClaim)
	if !ok {
		return errors.New("not string claim")
	}

	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.String != nil {
		c.String = *p.String
	}

	return nil
}

type AmountClaimPatch struct {
	Prop   *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Amount *float64               `exhaustruct:"optional" json:"amount,omitempty"`
	Unit   *AmountUnit            `exhaustruct:"optional" json:"unit,omitempty"`
}

func (p AmountClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || p.Amount == nil || p.Unit == nil {
		return nil, errors.New("incomplete patch")
	}

	return &AmountClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
		Amount: *p.Amount,
		Unit:   *p.Unit,
	}, nil
}

func (p AmountClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && p.Amount == nil && p.Unit == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*AmountClaim)
	if !ok {
		return errors.New("not amount claim")
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

type AmountRangeClaimPatch struct {
	Prop  *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Lower *float64               `exhaustruct:"optional" json:"lower,omitempty"`
	Upper *float64               `exhaustruct:"optional" json:"upper,omitempty"`
	Unit  *AmountUnit            `exhaustruct:"optional" json:"unit,omitempty"`
}

func (p AmountRangeClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || p.Lower == nil || p.Upper == nil || p.Unit == nil {
		return nil, errors.New("incomplete patch")
	}

	return &AmountRangeClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
		Lower: *p.Lower,
		Upper: *p.Upper,
		Unit:  *p.Unit,
	}, nil
}

func (p AmountRangeClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && p.Lower == nil && p.Upper == nil && p.Unit == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*AmountRangeClaim)
	if !ok {
		return errors.New("not amount range claim")
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

type RelationClaimPatch struct {
	Prop *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	To   *identifier.Identifier `exhaustruct:"optional" json:"to,omitempty"`
}

func (p RelationClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || p.To == nil {
		return nil, errors.New("incomplete patch")
	}

	return &RelationClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
		To: Reference{
			ID:    p.To,
			Score: 1.0, // TODO: Fetch if from the store?
		},
	}, nil
}

func (p RelationClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && p.To == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*RelationClaim)
	if !ok {
		return errors.New("not relation claim")
	}

	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.To != nil {
		c.To.ID = p.To
	}

	return nil
}

type FileClaimPatch struct {
	Prop    *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Type    *string                `exhaustruct:"optional" json:"type,omitempty"`
	URL     *string                `exhaustruct:"optional" json:"url,omitempty"`
	Preview []string               `exhaustruct:"optional" json:"preview"`
}

func (p FileClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || p.Type == nil || p.URL == nil || p.Preview == nil {
		return nil, errors.New("incomplete patch")
	}

	return &FileClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
		Type:    *p.Type,
		URL:     *p.URL,
		Preview: p.Preview,
	}, nil
}

func (p FileClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && p.Type == nil && p.URL == nil && p.Preview == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*FileClaim)
	if !ok {
		return errors.New("not file claim")
	}

	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}
	if p.Type != nil {
		c.Type = *p.Type
	}
	if p.URL != nil {
		c.URL = *p.URL
	}
	if p.Preview != nil {
		c.Preview = p.Preview
	}

	return nil
}

type NoValueClaimPatch struct {
	Prop *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
}

func (p NoValueClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil {
		return nil, errors.New("incomplete patch")
	}

	return &NoValueClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
	}, nil
}

func (p NoValueClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*NoValueClaim)
	if !ok {
		return errors.New("not no value claim")
	}

	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}

	return nil
}

func (p UnknownValueClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil {
		return nil, errors.New("incomplete patch")
	}

	return &UnknownValueClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
	}, nil
}

type UnknownValueClaimPatch struct {
	Prop *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
}

func (p UnknownValueClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*UnknownValueClaim)
	if !ok {
		return errors.New("not unknown value claim")
	}

	if p.Prop != nil {
		c.Prop.ID = p.Prop
	}

	return nil
}

type TimeClaimPatch struct {
	Prop      *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Timestamp *Timestamp             `exhaustruct:"optional" json:"timestamp,omitempty"`
	Precision *TimePrecision         `exhaustruct:"optional" json:"precision,omitempty"`
}

func (p TimeClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || p.Timestamp == nil || p.Precision == nil {
		return nil, errors.New("incomplete patch")
	}

	return &TimeClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
		Timestamp: *p.Timestamp,
		Precision: *p.Precision,
	}, nil
}

func (p TimeClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && p.Timestamp == nil && p.Precision == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*TimeClaim)
	if !ok {
		return errors.New("not time claim")
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

type TimeRangeClaimPatch struct {
	Prop      *identifier.Identifier `exhaustruct:"optional" json:"prop,omitempty"`
	Lower     *Timestamp             `exhaustruct:"optional" json:"lower,omitempty"`
	Upper     *Timestamp             `exhaustruct:"optional" json:"upper,omitempty"`
	Precision *TimePrecision         `exhaustruct:"optional" json:"precision,omitempty"`
}

func (p TimeRangeClaimPatch) New(id identifier.Identifier) (Claim, errors.E) { //nolint:ireturn
	if p.Prop == nil || p.Lower == nil || p.Upper == nil || p.Precision == nil {
		return nil, errors.New("incomplete patch")
	}

	return &TimeRangeClaim{
		CoreClaim: CoreClaim{
			ID:         id,
			Confidence: 1.0, // TODO How to make it configurable?
		},
		Prop: Reference{
			ID:    p.Prop,
			Score: 1.0, // TODO: Fetch if from the store?
		},
		Lower:     *p.Lower,
		Upper:     *p.Upper,
		Precision: *p.Precision,
	}, nil
}

func (p TimeRangeClaimPatch) Apply(claim Claim) errors.E {
	if p.Prop == nil && p.Lower == nil && p.Upper == nil && p.Precision == nil {
		return errors.New("empty patch")
	}

	c, ok := claim.(*TimeRangeClaim)
	if !ok {
		return errors.New("not time range claim")
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
