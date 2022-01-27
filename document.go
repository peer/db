package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"gitlab.com/tozd/go/errors"
)

type VisitResult int

const (
	Keep VisitResult = iota
	Drop
)

type Claim interface {
	GetID() Identifier
	GetConfidence() Confidence
	AddMeta(claim Claim) errors.E
	GetMetaByID(id Identifier) Claim
	RemoveMetaByID(id Identifier) Claim
	VisitMeta(visitor visitor) errors.E
}

type visitor interface {
	VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E)
	VisitReference(claim *ReferenceClaim) (VisitResult, errors.E)
	VisitText(claim *TextClaim) (VisitResult, errors.E)
	VisitString(claim *StringClaim) (VisitResult, errors.E)
	VisitLabel(claim *LabelClaim) (VisitResult, errors.E)
	VisitAmount(claim *AmountClaim) (VisitResult, errors.E)
	VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E)
	VisitEnumeration(claim *EnumerationClaim) (VisitResult, errors.E)
	VisitRelation(claim *RelationClaim) (VisitResult, errors.E)
	VisitFile(claim *FileClaim) (VisitResult, errors.E)
	VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E)
	VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E)
	VisitTime(claim *TimeClaim) (VisitResult, errors.E)
	VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E)
	VisitDuration(claim *DurationClaim) (VisitResult, errors.E)
	VisitDurationRange(claim *DurationRangeClaim) (VisitResult, errors.E)
	VisitList(claim *ListClaim) (VisitResult, errors.E)
}

type Document struct {
	CoreDocument

	Mnemonic Mnemonic    `json:"mnemonic,omitempty"`
	Active   *ClaimTypes `json:"active,omitempty"`
	Inactive *ClaimTypes `json:"inactive,omitempty"`
}

func (d *Document) Visit(visitor visitor) errors.E {
	if d.Active != nil {
		err := d.Active.Visit(visitor)
		if err != nil {
			return err
		}
		// If active claims became empty after visiting, we set them to nil.
		if d.Active.Size() == 0 {
			d.Active = nil
		}
	}
	if d.Inactive != nil {
		err := d.Inactive.Visit(visitor)
		if err != nil {
			return err
		}
		// If inactive claims became empty after visiting, we set them to nil.
		if d.Inactive.Size() == 0 {
			d.Inactive = nil
		}
	}
	return nil
}

func (c *ClaimTypes) Visit(visitor visitor) errors.E {
	if c == nil {
		return nil
	}

	k := 0
	for i, claim := range c.Identifier {
		keep, err := visitor.VisitIdentifier(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.Identifier[k] = claim
			}
			k++
		}
	}
	if len(c.Identifier) != k {
		c.Identifier = c.Identifier[:k]
	}

	k = 0
	for i, claim := range c.Reference {
		keep, err := visitor.VisitReference(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.Reference[k] = claim
			}
			k++
		}
	}
	if len(c.Reference) != k {
		c.Reference = c.Reference[:k]
	}

	k = 0
	for i, claim := range c.Text {
		keep, err := visitor.VisitText(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.Text[k] = claim
			}
			k++
		}
	}
	if len(c.Text) != k {
		c.Text = c.Text[:k]
	}

	k = 0
	for i, claim := range c.String {
		keep, err := visitor.VisitString(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.String[k] = claim
			}
			k++
		}
	}
	if len(c.String) != k {
		c.String = c.String[:k]
	}

	k = 0
	for i, claim := range c.Label {
		keep, err := visitor.VisitLabel(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.Label[k] = claim
			}
			k++
		}
	}
	if len(c.Label) != k {
		c.Label = c.Label[:k]
	}

	k = 0
	for i, claim := range c.Amount {
		keep, err := visitor.VisitAmount(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.Amount[k] = claim
			}
			k++
		}
	}
	if len(c.Amount) != k {
		c.Amount = c.Amount[:k]
	}

	k = 0
	for i, claim := range c.AmountRange {
		keep, err := visitor.VisitAmountRange(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.AmountRange[k] = claim
			}
			k++
		}
	}
	if len(c.AmountRange) != k {
		c.AmountRange = c.AmountRange[:k]
	}

	k = 0
	for i, claim := range c.Enumeration {
		keep, err := visitor.VisitEnumeration(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.Enumeration[k] = claim
			}
			k++
		}
	}
	if len(c.Enumeration) != k {
		c.Enumeration = c.Enumeration[:k]
	}

	k = 0
	for i, claim := range c.Relation {
		keep, err := visitor.VisitRelation(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.Relation[k] = claim
			}
			k++
		}
	}
	if len(c.Relation) != k {
		c.Relation = c.Relation[:k]
	}

	k = 0
	for i, claim := range c.File {
		keep, err := visitor.VisitFile(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.File[k] = claim
			}
			k++
		}
	}
	if len(c.File) != k {
		c.File = c.File[:k]
	}

	k = 0
	for i, claim := range c.NoValue {
		keep, err := visitor.VisitNoValue(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.NoValue[k] = claim
			}
			k++
		}
	}
	if len(c.NoValue) != k {
		c.NoValue = c.NoValue[:k]
	}

	k = 0
	for i, claim := range c.UnknownValue {
		keep, err := visitor.VisitUnknownValue(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.UnknownValue[k] = claim
			}
			k++
		}
	}
	if len(c.UnknownValue) != k {
		c.UnknownValue = c.UnknownValue[:k]
	}

	k = 0
	for i, claim := range c.Time {
		keep, err := visitor.VisitTime(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.Time[k] = claim
			}
			k++
		}
	}
	if len(c.Time) != k {
		c.Time = c.Time[:k]
	}

	k = 0
	for i, claim := range c.TimeRange {
		keep, err := visitor.VisitTimeRange(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.TimeRange[k] = claim
			}
			k++
		}
	}
	if len(c.TimeRange) != k {
		c.TimeRange = c.TimeRange[:k]
	}

	k = 0
	for i, claim := range c.Duration {
		keep, err := visitor.VisitDuration(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.Duration[k] = claim
			}
			k++
		}
	}
	if len(c.Duration) != k {
		c.Duration = c.Duration[:k]
	}

	k = 0
	for i, claim := range c.DurationRange {
		keep, err := visitor.VisitDurationRange(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.DurationRange[k] = claim
			}
			k++
		}
	}
	if len(c.DurationRange) != k {
		c.DurationRange = c.DurationRange[:k]
	}

	k = 0
	for i, claim := range c.List {
		keep, err := visitor.VisitList(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				c.List[k] = claim
			}
			k++
		}
	}
	if len(c.List) != k {
		c.List = c.List[:k]
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
	s += len(c.Label)
	s += len(c.Amount)
	s += len(c.AmountRange)
	s += len(c.Enumeration)
	s += len(c.Relation)
	s += len(c.File)
	s += len(c.NoValue)
	s += len(c.UnknownValue)
	s += len(c.Time)
	s += len(c.TimeRange)
	s += len(c.Duration)
	s += len(c.DurationRange)
	s += len(c.List)
	return s
}

type getByIDVisitor struct {
	ID     Identifier
	Action VisitResult
	Result Claim
}

var getByIDVisitorStopError = errors.Base("stop visitor")

func (v *getByIDVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitText(claim *TextClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitLabel(claim *LabelClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitEnumeration(claim *EnumerationClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitFile(claim *FileClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitDuration(claim *DurationClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitDurationRange(claim *DurationRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitList(claim *ListClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (d *Document) GetByID(id Identifier) Claim {
	v := getByIDVisitor{
		ID:     id,
		Action: Keep,
		Result: nil,
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *Document) RemoveByID(id Identifier) Claim {
	v := getByIDVisitor{
		ID:     id,
		Action: Drop,
		Result: nil,
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *Document) Add(claim Claim) errors.E {
	if claimID := claim.GetID(); d.GetByID(claimID) != nil {
		return errors.Errorf(`claim with ID "%s" already exists`, claimID)
	}
	activeClaims := claim.GetConfidence() >= 0.0
	switch c := claim.(type) {
	case *AmountClaim:
		activeClaims = activeClaims && c.Unit != AmountUnitCustom
	case *AmountRangeClaim:
		activeClaims = activeClaims && c.Unit != AmountUnitCustom
	}
	var claimTypes *ClaimTypes
	if activeClaims {
		if d.Active == nil {
			d.Active = &ClaimTypes{}
		}
		claimTypes = d.Active
	} else {
		if d.Inactive == nil {
			d.Inactive = &ClaimTypes{}
		}
		claimTypes = d.Inactive
	}
	switch c := claim.(type) {
	case *IdentifierClaim:
		claimTypes.Identifier = append(claimTypes.Identifier, *c)
	case *ReferenceClaim:
		claimTypes.Reference = append(claimTypes.Reference, *c)
	case *TextClaim:
		claimTypes.Text = append(claimTypes.Text, *c)
	case *StringClaim:
		claimTypes.String = append(claimTypes.String, *c)
	case *LabelClaim:
		claimTypes.Label = append(claimTypes.Label, *c)
	case *AmountClaim:
		claimTypes.Amount = append(claimTypes.Amount, *c)
	case *AmountRangeClaim:
		claimTypes.AmountRange = append(claimTypes.AmountRange, *c)
	case *EnumerationClaim:
		claimTypes.Enumeration = append(claimTypes.Enumeration, *c)
	case *RelationClaim:
		claimTypes.Relation = append(claimTypes.Relation, *c)
	case *FileClaim:
		claimTypes.File = append(claimTypes.File, *c)
	case *NoValueClaim:
		claimTypes.NoValue = append(claimTypes.NoValue, *c)
	case *UnknownValueClaim:
		claimTypes.UnknownValue = append(claimTypes.UnknownValue, *c)
	case *TimeClaim:
		claimTypes.Time = append(claimTypes.Time, *c)
	case *TimeRangeClaim:
		claimTypes.TimeRange = append(claimTypes.TimeRange, *c)
	case *DurationClaim:
		claimTypes.Duration = append(claimTypes.Duration, *c)
	case *DurationRangeClaim:
		claimTypes.DurationRange = append(claimTypes.DurationRange, *c)
	case *ListClaim:
		claimTypes.List = append(claimTypes.List, *c)
	default:
		return errors.Errorf(`claim of type %T is not supported`, claim)
	}
	return nil
}

type CoreDocument struct {
	ID         Identifier `json:"-"`
	Name       Name       `json:"name"`
	OtherNames OtherNames `json:"otherNames,omitempty"`
	Score      Score      `json:"score"`
	Scores     Scores     `json:"scores,omitempty"`
}

type Mnemonic string

type Identifier string

type Timestamp time.Time

var timeRegex = regexp.MustCompile(`^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$`)

func (t Timestamp) MarshalJSON() ([]byte, error) {
	x := time.Time(t).UTC()
	w := 4
	if x.Year() < 0 {
		// An extra character for the minus sign.
		w = 5
	}
	return []byte(fmt.Sprintf(`"%0*d-%02d-%02dT%02d:%02d:%02dZ"`, w, x.Year(), x.Month(), x.Day(), x.Hour(), x.Minute(), x.Second())), nil
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
	year, err := strconv.ParseInt(match[1], 10, 0) //nolint:gomnd
	if err != nil {
		return errors.WithMessagef(err, `unable to parse year "%s"`, s)
	}
	month, err := strconv.ParseInt(match[2], 10, 0) //nolint:gomnd
	if err != nil {
		return errors.WithMessagef(err, `unable to parse month "%s"`, s)
	}
	day, err := strconv.ParseInt(match[3], 10, 0) //nolint:gomnd
	if err != nil {
		return errors.WithMessagef(err, `unable to parse day "%s"`, s)
	}
	hour, err := strconv.ParseInt(match[4], 10, 0) //nolint:gomnd
	if err != nil {
		return errors.WithMessagef(err, `unable to parse hour "%s"`, s)
	}
	minute, err := strconv.ParseInt(match[5], 10, 0) //nolint:gomnd
	if err != nil {
		return errors.WithMessagef(err, `unable to parse minute "%s"`, s)
	}
	second, err := strconv.ParseInt(match[6], 10, 0) //nolint:gomnd
	if err != nil {
		return errors.WithMessagef(err, `unable to parse second "%s"`, s)
	}
	*t = Timestamp(time.Date(int(year), time.Month(month), int(day), int(hour), int(minute), int(second), 0, time.UTC))
	return nil
}

type Duration time.Duration

type Name = TranslatablePlainString

// Language to plain string mapping.
type TranslatablePlainString map[string]string

// Language to HTML string mapping.
type TranslatableHTMLString map[string]string

// Language to string slice mapping.
type OtherNames map[string][]string

// Score name to score mapping.
type Scores map[string]Score

type ClaimTypes struct {
	Identifier    IdentifierClaims    `json:"id,omitempty"`
	Reference     ReferenceClaims     `json:"ref,omitempty"`
	Text          TextClaims          `json:"text,omitempty"`
	String        StringClaims        `json:"string,omitempty"`
	Label         LabelClaims         `json:"label,omitempty"`
	Amount        AmountClaims        `json:"amount,omitempty"`
	AmountRange   AmountRangeClaims   `json:"amountRange,omitempty"`
	Enumeration   EnumerationClaims   `json:"enum,omitempty"`
	Relation      RelationClaims      `json:"rel,omitempty"`
	File          FileClaims          `json:"file,omitempty"`
	NoValue       NoValueClaims       `json:"none,omitempty"`
	UnknownValue  UnknownValueClaims  `json:"unknown,omitempty"`
	Time          TimeClaims          `json:"time,omitempty"`
	TimeRange     TimeRangeClaims     `json:"timeRange,omitempty"`
	Duration      DurationClaims      `json:"duration,omitempty"`
	DurationRange DurationRangeClaims `json:"durationRange,omitempty"`
	List          ListClaims          `json:"list,omitempty"`
}

type (
	IdentifierClaims    = []IdentifierClaim
	ReferenceClaims     = []ReferenceClaim
	TextClaims          = []TextClaim
	StringClaims        = []StringClaim
	LabelClaims         = []LabelClaim
	AmountClaims        = []AmountClaim
	AmountRangeClaims   = []AmountRangeClaim
	EnumerationClaims   = []EnumerationClaim
	RelationClaims      = []RelationClaim
	FileClaims          = []FileClaim
	NoValueClaims       = []NoValueClaim
	UnknownValueClaims  = []UnknownValueClaim
	TimeClaims          = []TimeClaim
	TimeRangeClaims     = []TimeRangeClaim
	DurationClaims      = []DurationClaim
	DurationRangeClaims = []DurationRangeClaim
	ListClaims          = []ListClaim
)

type CoreClaim struct {
	ID         Identifier  `json:"_id"`
	Confidence Confidence  `json:"confidence"`
	Meta       *ClaimTypes `json:"meta,omitempty"`
}

func (cc CoreClaim) GetID() Identifier {
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
		cc.Meta = &ClaimTypes{}
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
	case *LabelClaim:
		cc.Meta.Label = append(cc.Meta.Label, *c)
	case *AmountClaim:
		cc.Meta.Amount = append(cc.Meta.Amount, *c)
	case *AmountRangeClaim:
		cc.Meta.AmountRange = append(cc.Meta.AmountRange, *c)
	case *EnumerationClaim:
		cc.Meta.Enumeration = append(cc.Meta.Enumeration, *c)
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
	case *DurationClaim:
		cc.Meta.Duration = append(cc.Meta.Duration, *c)
	case *DurationRangeClaim:
		cc.Meta.DurationRange = append(cc.Meta.DurationRange, *c)
	case *ListClaim:
		cc.Meta.List = append(cc.Meta.List, *c)
	default:
		return errors.Errorf(`meta claim of type %T is not supported`, claim)
	}
	return nil
}

func (cc *CoreClaim) VisitMeta(visitor visitor) errors.E {
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

func (cc *CoreClaim) GetMetaByID(id Identifier) Claim {
	v := getByIDVisitor{
		ID:     id,
		Result: nil,
		Action: Keep,
	}
	_ = cc.VisitMeta(&v)
	return v.Result
}

func (cc *CoreClaim) RemoveMetaByID(id Identifier) Claim {
	v := getByIDVisitor{
		ID:     id,
		Result: nil,
		Action: Drop,
	}
	_ = cc.VisitMeta(&v)
	return v.Result
}

type Confidence = Score

type Score float64

type DocumentReference struct {
	ID     Identifier `json:"_id"`
	Name   Name       `json:"name"`
	Score  Score      `json:"score"`
	Scores Scores     `json:"scores,omitempty"`
}

type IdentifierClaim struct {
	CoreClaim

	Prop       DocumentReference `json:"prop"`
	Identifier string            `json:"id"`
}

type ReferenceClaim struct {
	CoreClaim

	Prop DocumentReference `json:"prop"`
	IRI  string            `json:"iri"`
}

type TextClaim struct {
	CoreClaim

	Prop DocumentReference      `json:"prop"`
	HTML TranslatableHTMLString `json:"html"`
}

type StringClaim struct {
	CoreClaim

	Prop   DocumentReference `json:"prop"`
	String string            `json:"string"`
}

type LabelClaim struct {
	CoreClaim

	Prop DocumentReference `json:"prop"`
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
	default:
		return errors.Errorf("unknown amount unit: %s", s)
	}
	return nil
}

type AmountClaim struct {
	CoreClaim

	Prop             DocumentReference `json:"prop"`
	Amount           float64           `json:"amount"`
	UncertaintyLower *float64          `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper *float64          `json:"uncertaintyUpper,omitempty"`
	Unit             AmountUnit        `json:"unit"`
}

type AmountRangeClaim struct {
	CoreClaim

	Prop             DocumentReference `json:"prop"`
	Lower            float64           `json:"lower"`
	Upper            float64           `json:"upper"`
	UncertaintyLower *float64          `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper *float64          `json:"uncertaintyUpper,omitempty"`
	Unit             AmountUnit        `json:"unit"`
}

type EnumerationClaim struct {
	CoreClaim

	Prop DocumentReference `json:"prop"`
	Enum []string          `json:"enum"`
}

type RelationClaim struct {
	CoreClaim

	Prop DocumentReference `json:"prop"`
	To   DocumentReference `json:"to"`
}

type FileClaim struct {
	CoreClaim

	Prop    DocumentReference `json:"prop"`
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Preview []string          `json:"preview,omitempty"`
}

type NoValueClaim struct {
	CoreClaim

	Prop DocumentReference `json:"prop"`
}

type UnknownValueClaim struct {
	CoreClaim

	Prop DocumentReference `json:"prop"`
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

	Prop             DocumentReference `json:"prop"`
	Timestamp        Timestamp         `json:"timestamp"`
	UncertaintyLower *Timestamp        `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper *Timestamp        `json:"uncertaintyUpper,omitempty"`
	Precision        TimePrecision     `json:"precision"`
}

type TimeRangeClaim struct {
	CoreClaim

	Prop             DocumentReference `json:"prop"`
	Lower            Timestamp         `json:"lower"`
	Upper            Timestamp         `json:"upper"`
	UncertaintyLower *Timestamp        `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper *Timestamp        `json:"uncertaintyUpper,omitempty"`
	Precision        TimePrecision     `json:"precision"`
}

type DurationClaim struct {
	CoreClaim

	Prop             DocumentReference `json:"prop"`
	Amount           Duration          `json:"amount"`
	UncertaintyLower *Duration         `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper *Duration         `json:"uncertaintyUpper,omitempty"`
}

type DurationRangeClaim struct {
	CoreClaim

	Prop             DocumentReference `json:"prop"`
	Lower            Duration          `json:"lower"`
	Upper            Duration          `json:"upper"`
	UncertaintyLower *Duration         `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper *Duration         `json:"uncertaintyUpper,omitempty"`
}

type ListClaim struct {
	CoreClaim

	Prop     DocumentReference `json:"prop"`
	Element  DocumentReference `json:"el"`
	List     Identifier        `json:"list"`
	Order    float64           `json:"order"`
	Children []ListChild       `json:"children,omitempty"`
}

type ListChild struct {
	Prop  DocumentReference `json:"prop"`
	Child Identifier        `json:"child"`
}
