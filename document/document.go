package document

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

type VisitResult int

const (
	Keep VisitResult = iota
	KeepAndStop
	Drop
	DropAndStop
)

type Claim interface {
	GetID() identifier.Identifier
	GetConfidence() Confidence
	AddMeta(claim Claim) errors.E
	GetMetaByID(id identifier.Identifier) Claim
	RemoveMetaByID(id identifier.Identifier) Claim
	VisitMeta(visitor Visitor) errors.E
}

type Visitor interface { //nolint:interfacebloat
	VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E)
	VisitReference(claim *ReferenceClaim) (VisitResult, errors.E)
	VisitText(claim *TextClaim) (VisitResult, errors.E)
	VisitString(claim *StringClaim) (VisitResult, errors.E)
	VisitAmount(claim *AmountClaim) (VisitResult, errors.E)
	VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E)
	VisitRelation(claim *RelationClaim) (VisitResult, errors.E)
	VisitFile(claim *FileClaim) (VisitResult, errors.E)
	VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E)
	VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E)
	VisitTime(claim *TimeClaim) (VisitResult, errors.E)
	VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E)
}

func (c *ClaimTypes) Visit(visitor Visitor) errors.E { //nolint:maintidx
	if c == nil {
		return nil
	}

	var err errors.E

	stopping := false
	k := 0
	for i := range c.Identifier {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitIdentifier(&c.Identifier[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Identifier[k] = c.Identifier[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Identifier) != k {
		c.Identifier = c.Identifier[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Reference {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitReference(&c.Reference[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Reference[k] = c.Reference[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Reference) != k {
		c.Reference = c.Reference[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Text {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitText(&c.Text[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Text[k] = c.Text[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Text) != k {
		c.Text = c.Text[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.String {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitString(&c.String[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.String[k] = c.String[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.String) != k {
		c.String = c.String[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Amount {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitAmount(&c.Amount[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Amount[k] = c.Amount[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Amount) != k {
		c.Amount = c.Amount[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.AmountRange {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitAmountRange(&c.AmountRange[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.AmountRange[k] = c.AmountRange[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.AmountRange) != k {
		c.AmountRange = c.AmountRange[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Relation {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitRelation(&c.Relation[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Relation[k] = c.Relation[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Relation) != k {
		c.Relation = c.Relation[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.File {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitFile(&c.File[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.File[k] = c.File[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.File) != k {
		c.File = c.File[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.NoValue {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitNoValue(&c.NoValue[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.NoValue[k] = c.NoValue[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.NoValue) != k {
		c.NoValue = c.NoValue[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.UnknownValue {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitUnknownValue(&c.UnknownValue[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.UnknownValue[k] = c.UnknownValue[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.UnknownValue) != k {
		c.UnknownValue = c.UnknownValue[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Time {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitTime(&c.Time[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Time[k] = c.Time[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Time) != k {
		c.Time = c.Time[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.TimeRange {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitTimeRange(&c.TimeRange[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.TimeRange[k] = c.TimeRange[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.TimeRange) != k {
		c.TimeRange = c.TimeRange[:k]
	}
	if stopping {
		return nil
	}

	return nil
}

func (c *ClaimTypes) Size() int {
	if c == nil {
		return 0
	}

	s := 0
	s += len(c.Identifier)
	s += len(c.Reference)
	s += len(c.Text)
	s += len(c.String)
	s += len(c.Amount)
	s += len(c.AmountRange)
	s += len(c.Relation)
	s += len(c.File)
	s += len(c.NoValue)
	s += len(c.UnknownValue)
	s += len(c.Time)
	s += len(c.TimeRange)
	return s
}

type GetByIDVisitor struct {
	ID     identifier.Identifier
	Action VisitResult
	Result Claim
}

func (v *GetByIDVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitText(claim *TextClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitFile(claim *FileClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByIDVisitor) VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	return Keep, nil
}

type GetByPropIDVisitor struct {
	ID     identifier.Identifier
	Action VisitResult
	Result []Claim
}

func (v *GetByPropIDVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitText(claim *TextClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitFile(claim *FileClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

type AllClaimsVisitor struct {
	Result []Claim
}

func (v *AllClaimsVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitText(claim *TextClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitFile(claim *FileClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

type CoreDocument struct {
	ID     identifier.Identifier `                       json:"id"`
	Score  Score                 `                       json:"score"`
	Scores Scores                `exhaustruct:"optional" json:"scores,omitempty"`
}

type Mnemonic string

type Timestamp time.Time

var timeRegex = regexp.MustCompile(`^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$`)

func (t Timestamp) MarshalJSON() ([]byte, error) {
	b := bytes.Buffer{}
	b.WriteString(`"`)
	b.WriteString(t.String())
	b.WriteString(`"`)
	return b.Bytes(), nil
}

func (t Timestamp) String() string {
	x := time.Time(t).UTC()
	w := 4
	if x.Year() < 0 {
		// An extra character for the minus sign.
		w = 5
	}
	year, month, day := x.Date()
	return fmt.Sprintf(`%0*d-%02d-%02dT%02d:%02d:%02dZ`, w, year, month, day, x.Hour(), x.Minute(), x.Second())
}

// We cannot use standard time.Time implementation.
// See: https://github.com/golang/go/issues/4556

func (t *Timestamp) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return errors.WithStack(err)
	}
	match := timeRegex.FindStringSubmatch(s)
	if match == nil {
		return errors.Errorf(`unable to parse time "%s"`, s)
	}
	year, err := strconv.ParseInt(match[1], 10, 0)
	if err != nil {
		return errors.WithMessagef(err, `unable to parse year "%s"`, s)
	}
	month, err := strconv.ParseInt(match[2], 10, 0)
	if err != nil {
		return errors.WithMessagef(err, `unable to parse month "%s"`, s)
	}
	day, err := strconv.ParseInt(match[3], 10, 0)
	if err != nil {
		return errors.WithMessagef(err, `unable to parse day "%s"`, s)
	}
	hour, err := strconv.ParseInt(match[4], 10, 0)
	if err != nil {
		return errors.WithMessagef(err, `unable to parse hour "%s"`, s)
	}
	minute, err := strconv.ParseInt(match[5], 10, 0)
	if err != nil {
		return errors.WithMessagef(err, `unable to parse minute "%s"`, s)
	}
	second, err := strconv.ParseInt(match[6], 10, 0)
	if err != nil {
		return errors.WithMessagef(err, `unable to parse second "%s"`, s)
	}
	*t = Timestamp(time.Date(int(year), time.Month(month), int(day), int(hour), int(minute), int(second), 0, time.UTC))
	return nil
}

// Language to HTML string mapping.
type TranslatableHTMLString map[string]string

// Score name to score mapping.
type Scores map[string]Score

type ClaimTypes struct {
	Identifier   IdentifierClaims   `json:"id,omitempty"`
	Reference    ReferenceClaims    `json:"ref,omitempty"`
	Text         TextClaims         `json:"text,omitempty"`
	String       StringClaims       `json:"string,omitempty"`
	Amount       AmountClaims       `json:"amount,omitempty"`
	AmountRange  AmountRangeClaims  `json:"amountRange,omitempty"`
	Relation     RelationClaims     `json:"rel,omitempty"`
	File         FileClaims         `json:"file,omitempty"`
	NoValue      NoValueClaims      `json:"none,omitempty"`
	UnknownValue UnknownValueClaims `json:"unknown,omitempty"`
	Time         TimeClaims         `json:"time,omitempty"`
	TimeRange    TimeRangeClaims    `json:"timeRange,omitempty"`
}

type (
	IdentifierClaims   = []IdentifierClaim
	ReferenceClaims    = []ReferenceClaim
	TextClaims         = []TextClaim
	StringClaims       = []StringClaim
	AmountClaims       = []AmountClaim
	AmountRangeClaims  = []AmountRangeClaim
	RelationClaims     = []RelationClaim
	FileClaims         = []FileClaim
	NoValueClaims      = []NoValueClaim
	UnknownValueClaims = []UnknownValueClaim
	TimeClaims         = []TimeClaim
	TimeRangeClaims    = []TimeRangeClaim
)

type CoreClaim struct {
	ID         identifier.Identifier `                       json:"id"`
	Confidence Confidence            `                       json:"confidence"`
	Meta       *ClaimTypes           `exhaustruct:"optional" json:"meta,omitempty"`
}

func (cc CoreClaim) GetID() identifier.Identifier {
	return cc.ID
}

func (cc CoreClaim) GetConfidence() Confidence {
	return cc.Confidence
}

func (cc *CoreClaim) AddMeta(claim Claim) errors.E {
	if claimID := claim.GetID(); cc.GetMetaByID(claimID) != nil {
		return errors.Errorf(`meta claim with ID "%s" already exists`, claimID)
	}
	if cc.Meta == nil {
		cc.Meta = new(ClaimTypes)
	}
	switch c := claim.(type) {
	case *IdentifierClaim:
		cc.Meta.Identifier = append(cc.Meta.Identifier, *c)
	case *ReferenceClaim:
		cc.Meta.Reference = append(cc.Meta.Reference, *c)
	case *TextClaim:
		cc.Meta.Text = append(cc.Meta.Text, *c)
	case *StringClaim:
		cc.Meta.String = append(cc.Meta.String, *c)
	case *AmountClaim:
		cc.Meta.Amount = append(cc.Meta.Amount, *c)
	case *AmountRangeClaim:
		cc.Meta.AmountRange = append(cc.Meta.AmountRange, *c)
	case *RelationClaim:
		cc.Meta.Relation = append(cc.Meta.Relation, *c)
	case *FileClaim:
		cc.Meta.File = append(cc.Meta.File, *c)
	case *NoValueClaim:
		cc.Meta.NoValue = append(cc.Meta.NoValue, *c)
	case *UnknownValueClaim:
		cc.Meta.UnknownValue = append(cc.Meta.UnknownValue, *c)
	case *TimeClaim:
		cc.Meta.Time = append(cc.Meta.Time, *c)
	case *TimeRangeClaim:
		cc.Meta.TimeRange = append(cc.Meta.TimeRange, *c)
	default:
		return errors.Errorf(`meta claim of type %T is not supported`, claim)
	}
	return nil
}

func (cc *CoreClaim) VisitMeta(visitor Visitor) errors.E {
	if cc.Meta != nil {
		err := cc.Meta.Visit(visitor)
		if err != nil {
			return err
		}
		// If meta claims became empty after visiting, we set them to nil.
		if cc.Meta.Size() == 0 {
			cc.Meta = nil
		}
	}
	return nil
}

func (cc *CoreClaim) GetMetaByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Result: nil,
		Action: Keep,
	}
	_ = cc.VisitMeta(&v)
	return v.Result
}

func (cc *CoreClaim) GetMeta(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Keep,
		Result: []Claim{},
	}
	_ = cc.VisitMeta(&v)
	return v.Result
}

func (cc *CoreClaim) RemoveMetaByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Result: nil,
		Action: Drop,
	}
	_ = cc.VisitMeta(&v)
	return v.Result
}

type Confidence = Score

type Score float64

type Reference struct {
	ID    *identifier.Identifier `json:"id,omitempty"`
	Score Score                  `json:"score"`

	// Used to store temporary opaque reference before it is resolved in the second pass when importing data.
	Temporary []string `exhaustruct:"optional" json:"_temp,omitempty"` //nolint:tagliatelle
}

type IdentifierClaim struct {
	CoreClaim

	Prop       Reference `json:"prop"`
	Identifier string    `json:"id"`
}

type ReferenceClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
	IRI  string    `json:"iri"`
}

type TextClaim struct {
	CoreClaim

	Prop Reference              `json:"prop"`
	HTML TranslatableHTMLString `json:"html"`
}

type StringClaim struct {
	CoreClaim

	Prop   Reference `json:"prop"`
	String string    `json:"string"`
}

type AmountUnit int

const (
	AmountUnitCustom AmountUnit = iota
	AmountUnitNone
	AmountUnitRatio
	AmountUnitKilogramPerKilogram
	AmountUnitKilogram
	AmountUnitKilogramPerCubicMetre
	AmountUnitMetre
	AmountUnitSquareMetre
	AmountUnitMetrePerSecond
	AmountUnitVolt
	AmountUnitWatt
	AmountUnitPascal
	AmountUnitCoulomb
	AmountUnitJoule
	AmountUnitCelsius
	AmountUnitRadian
	AmountUnitHertz
	AmountUnitDollar
	AmountUnitByte
	AmountUnitPixel
	AmountUnitSecond

	// Count of the number of possible values.
	AmountUnitsTotal
)

func (u AmountUnit) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	switch u {
	case AmountUnitCustom:
		buffer.WriteString("@")
	case AmountUnitNone:
		buffer.WriteString("1")
	case AmountUnitRatio:
		buffer.WriteString("/")
	case AmountUnitKilogramPerKilogram:
		buffer.WriteString("kg/kg")
	case AmountUnitKilogram:
		buffer.WriteString("kg")
	case AmountUnitKilogramPerCubicMetre:
		buffer.WriteString("kg/m³")
	case AmountUnitMetre:
		buffer.WriteString("m")
	case AmountUnitSquareMetre:
		buffer.WriteString("m²")
	case AmountUnitMetrePerSecond:
		buffer.WriteString("m/s")
	case AmountUnitVolt:
		buffer.WriteString("V")
	case AmountUnitWatt:
		buffer.WriteString("W")
	case AmountUnitPascal:
		buffer.WriteString("Pa")
	case AmountUnitCoulomb:
		buffer.WriteString("C")
	case AmountUnitJoule:
		buffer.WriteString("J")
	case AmountUnitCelsius:
		buffer.WriteString("°C")
	case AmountUnitRadian:
		buffer.WriteString("rad")
	case AmountUnitHertz:
		buffer.WriteString("Hz")
	case AmountUnitDollar:
		buffer.WriteString("$")
	case AmountUnitByte:
		buffer.WriteString("B")
	case AmountUnitPixel:
		buffer.WriteString("px")
	case AmountUnitSecond:
		buffer.WriteString("s")
	case AmountUnitsTotal:
		fallthrough
	default:
		panic(errors.Errorf("invalid AmountUnit value: %d", u))
	}
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (u *AmountUnit) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return errors.WithStack(err)
	}
	switch s {
	case "@":
		*u = AmountUnitCustom
	case "1":
		*u = AmountUnitNone
	case "/":
		*u = AmountUnitRatio
	case "kg/kg":
		*u = AmountUnitKilogramPerKilogram
	case "kg":
		*u = AmountUnitKilogram
	case "kg/m³":
		*u = AmountUnitKilogramPerCubicMetre
	case "m":
		*u = AmountUnitMetre
	case "m²":
		*u = AmountUnitSquareMetre
	case "m/s":
		*u = AmountUnitMetrePerSecond
	case "V":
		*u = AmountUnitVolt
	case "W":
		*u = AmountUnitWatt
	case "Pa":
		*u = AmountUnitPascal
	case "C":
		*u = AmountUnitCoulomb
	case "J":
		*u = AmountUnitJoule
	case "°C":
		*u = AmountUnitCelsius
	case "rad":
		*u = AmountUnitRadian
	case "Hz":
		*u = AmountUnitHertz
	case "$":
		*u = AmountUnitDollar
	case "B":
		*u = AmountUnitByte
	case "px":
		*u = AmountUnitPixel
	case "s":
		*u = AmountUnitSecond
	default:
		return errors.Errorf("unknown amount unit: %s", s)
	}
	return nil
}

func ValidAmountUnit(unit string) bool {
	var u AmountUnit
	err := x.UnmarshalWithoutUnknownFields([]byte(`"`+unit+`"`), &u)
	return err == nil
}

type AmountClaim struct {
	CoreClaim

	Prop   Reference  `json:"prop"`
	Amount float64    `json:"amount"`
	Unit   AmountUnit `json:"unit"`
}

type AmountRangeClaim struct {
	CoreClaim

	Prop  Reference  `json:"prop"`
	Lower float64    `json:"lower"`
	Upper float64    `json:"upper"`
	Unit  AmountUnit `json:"unit"`
}

type RelationClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
	To   Reference `json:"to"`
}

type FileClaim struct {
	CoreClaim

	Prop    Reference `json:"prop"`
	Type    string    `json:"type"`
	URL     string    `json:"url"`
	Preview []string  `json:"preview,omitempty"`
}

type NoValueClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
}

type UnknownValueClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
}

type TimePrecision int

const (
	TimePrecisionGigaYears TimePrecision = iota
	TimePrecisionHundredMegaYears
	TimePrecisionTenMegaYears
	TimePrecisionMegaYears
	TimePrecisionHundredKiloYears
	TimePrecisionTenKiloYears
	TimePrecisionKiloYears
	TimePrecisionHundredYears
	TimePrecisionTenYears
	TimePrecisionYear
	TimePrecisionMonth
	TimePrecisionDay
	TimePrecisionHour
	TimePrecisionMinute
	TimePrecisionSecond
)

func (p TimePrecision) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	switch p {
	case TimePrecisionGigaYears:
		buffer.WriteString("G")
	case TimePrecisionHundredMegaYears:
		buffer.WriteString("100M")
	case TimePrecisionTenMegaYears:
		buffer.WriteString("10M")
	case TimePrecisionMegaYears:
		buffer.WriteString("M")
	case TimePrecisionHundredKiloYears:
		buffer.WriteString("100k")
	case TimePrecisionTenKiloYears:
		buffer.WriteString("10k")
	case TimePrecisionKiloYears:
		buffer.WriteString("k")
	case TimePrecisionHundredYears:
		buffer.WriteString("100y")
	case TimePrecisionTenYears:
		buffer.WriteString("10y")
	case TimePrecisionYear:
		buffer.WriteString("y")
	case TimePrecisionMonth:
		buffer.WriteString("m")
	case TimePrecisionDay:
		buffer.WriteString("d")
	case TimePrecisionHour:
		buffer.WriteString("h")
	case TimePrecisionMinute:
		buffer.WriteString("min")
	case TimePrecisionSecond:
		buffer.WriteString("s")
	}
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (p *TimePrecision) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return errors.WithStack(err)
	}
	switch s {
	case "G":
		*p = TimePrecisionGigaYears
	case "100M":
		*p = TimePrecisionHundredMegaYears
	case "10M":
		*p = TimePrecisionTenMegaYears
	case "M":
		*p = TimePrecisionMegaYears
	case "100k":
		*p = TimePrecisionHundredKiloYears
	case "10k":
		*p = TimePrecisionTenKiloYears
	case "k":
		*p = TimePrecisionKiloYears
	case "100y":
		*p = TimePrecisionHundredYears
	case "10y":
		*p = TimePrecisionTenYears
	case "y":
		*p = TimePrecisionYear
	case "m":
		*p = TimePrecisionMonth
	case "d":
		*p = TimePrecisionDay
	case "h":
		*p = TimePrecisionHour
	case "min":
		*p = TimePrecisionMinute
	case "s":
		*p = TimePrecisionSecond
	default:
		return errors.Errorf("unknown time precision: %s", s)
	}
	return nil
}

type TimeClaim struct {
	CoreClaim

	Prop      Reference     `json:"prop"`
	Timestamp Timestamp     `json:"timestamp"`
	Precision TimePrecision `json:"precision"`
}

type TimeRangeClaim struct {
	CoreClaim

	Prop      Reference     `json:"prop"`
	Lower     Timestamp     `json:"lower"`
	Upper     Timestamp     `json:"upper"`
	Precision TimePrecision `json:"precision"`
}
