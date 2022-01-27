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
	VisitMeta(visitor visitorMeta) errors.E
}

type visitorMeta interface {
	VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E)
	VisitReference(claim *ReferenceClaim) (VisitResult, errors.E)
	VisitText(claim *TextClaim) (VisitResult, errors.E)
	VisitString(claim *StringClaim) (VisitResult, errors.E)
	VisitLabel(claim *LabelClaim) (VisitResult, errors.E)
	VisitAmount(claim *AmountClaim) (VisitResult, errors.E)
	VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E)
	VisitEnumeration(claim *EnumerationClaim) (VisitResult, errors.E)
	VisitRelation(claim *RelationClaim) (VisitResult, errors.E)
	VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E)
	VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E)
	VisitTime(claim *TimeClaim) (VisitResult, errors.E)
	VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E)
	VisitDuration(claim *DurationClaim) (VisitResult, errors.E)
	VisitDurationRange(claim *DurationRangeClaim) (VisitResult, errors.E)
	VisitFile(claim *FileClaim) (VisitResult, errors.E)
}

type visitor interface {
	visitorMeta

	VisitList(claim *ListClaim) (VisitResult, errors.E)
}

type Document struct {
	CoreDocument

	Mnemonic Mnemonic            `json:"mnemonic,omitempty"`
	Active   *DocumentClaimTypes `json:"active,omitempty"`
	Inactive *DocumentClaimTypes `json:"inactive,omitempty"`
}

func (d *Document) Visit(visitor visitor) errors.E {
	for _, claims := range []*DocumentClaimTypes{d.Active, d.Inactive} {
		if claims == nil {
			continue
		}

		k := 0
		for i, claim := range claims.Identifier {
			keep, err := visitor.VisitIdentifier(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.Identifier[k] = claim
				}
				k++
			}
		}
		if len(claims.Identifier) != k {
			claims.Identifier = claims.Identifier[:k]
		}

		k = 0
		for i, claim := range claims.Reference {
			keep, err := visitor.VisitReference(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.Reference[k] = claim
				}
				k++
			}
		}
		if len(claims.Reference) != k {
			claims.Reference = claims.Reference[:k]
		}

		k = 0
		for i, claim := range claims.Text {
			keep, err := visitor.VisitText(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.Text[k] = claim
				}
				k++
			}
		}
		if len(claims.Text) != k {
			claims.Text = claims.Text[:k]
		}

		k = 0
		for i, claim := range claims.String {
			keep, err := visitor.VisitString(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.String[k] = claim
				}
				k++
			}
		}
		if len(claims.String) != k {
			claims.String = claims.String[:k]
		}

		k = 0
		for i, claim := range claims.Label {
			keep, err := visitor.VisitLabel(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.Label[k] = claim
				}
				k++
			}
		}
		if len(claims.Label) != k {
			claims.Label = claims.Label[:k]
		}

		k = 0
		for i, claim := range claims.Amount {
			keep, err := visitor.VisitAmount(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.Amount[k] = claim
				}
				k++
			}
		}
		if len(claims.Amount) != k {
			claims.Amount = claims.Amount[:k]
		}

		k = 0
		for i, claim := range claims.AmountRange {
			keep, err := visitor.VisitAmountRange(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.AmountRange[k] = claim
				}
				k++
			}
		}
		if len(claims.AmountRange) != k {
			claims.AmountRange = claims.AmountRange[:k]
		}

		k = 0
		for i, claim := range claims.Enumeration {
			keep, err := visitor.VisitEnumeration(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.Enumeration[k] = claim
				}
				k++
			}
		}
		if len(claims.Enumeration) != k {
			claims.Enumeration = claims.Enumeration[:k]
		}

		k = 0
		for i, claim := range claims.Relation {
			keep, err := visitor.VisitRelation(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.Relation[k] = claim
				}
				k++
			}
		}
		if len(claims.Relation) != k {
			claims.Relation = claims.Relation[:k]
		}

		k = 0
		for i, claim := range claims.NoValue {
			keep, err := visitor.VisitNoValue(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.NoValue[k] = claim
				}
				k++
			}
		}
		if len(claims.NoValue) != k {
			claims.NoValue = claims.NoValue[:k]
		}

		k = 0
		for i, claim := range claims.UnknownValue {
			keep, err := visitor.VisitUnknownValue(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.UnknownValue[k] = claim
				}
				k++
			}
		}
		if len(claims.UnknownValue) != k {
			claims.UnknownValue = claims.UnknownValue[:k]
		}

		k = 0
		for i, claim := range claims.Time {
			keep, err := visitor.VisitTime(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.Time[k] = claim
				}
				k++
			}
		}
		if len(claims.Time) != k {
			claims.Time = claims.Time[:k]
		}

		k = 0
		for i, claim := range claims.TimeRange {
			keep, err := visitor.VisitTimeRange(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.TimeRange[k] = claim
				}
				k++
			}
		}
		if len(claims.TimeRange) != k {
			claims.TimeRange = claims.TimeRange[:k]
		}

		k = 0
		for i, claim := range claims.Duration {
			keep, err := visitor.VisitDuration(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.Duration[k] = claim
				}
				k++
			}
		}
		if len(claims.Duration) != k {
			claims.Duration = claims.Duration[:k]
		}

		k = 0
		for i, claim := range claims.DurationRange {
			keep, err := visitor.VisitDurationRange(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.DurationRange[k] = claim
				}
				k++
			}
		}
		if len(claims.DurationRange) != k {
			claims.DurationRange = claims.DurationRange[:k]
		}

		k = 0
		for i, claim := range claims.File {
			keep, err := visitor.VisitFile(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.File[k] = claim
				}
				k++
			}
		}
		if len(claims.File) != k {
			claims.File = claims.File[:k]
		}

		k = 0
		for i, claim := range claims.List {
			keep, err := visitor.VisitList(&claim)
			if err != nil {
				return err
			}
			if keep == Keep {
				if i != k {
					claims.List[k] = claim
				}
				k++
			}
		}
		if len(claims.List) != k {
			claims.List = claims.List[:k]
		}
	}

	return nil
}

type getByIDVisitor struct {
	ID     Identifier
	Result Claim
}

var getByIDVisitorStopError = errors.Base("stop visitor")

func (v *getByIDVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitText(claim *TextClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitLabel(claim *LabelClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitEnumeration(claim *EnumerationClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitDuration(claim *DurationClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitDurationRange(claim *DurationRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitFile(claim *FileClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (v *getByIDVisitor) VisitList(claim *ListClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return Keep, errors.WithStack(getByIDVisitorStopError)
	}
	return Keep, nil
}

func (d *Document) GetByID(id Identifier) Claim {
	v := getByIDVisitor{
		ID:     id,
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
	var claimTypes *DocumentClaimTypes
	if activeClaims {
		if d.Active == nil {
			d.Active = &DocumentClaimTypes{}
		}
		claimTypes = d.Active
	} else {
		if d.Inactive == nil {
			d.Inactive = &DocumentClaimTypes{}
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
	case *FileClaim:
		claimTypes.File = append(claimTypes.File, *c)
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

type DocumentClaimTypes struct {
	RefClaimTypes
	SimpleClaimTypes
	TimeClaimTypes

	List ListClaims `json:"list,omitempty"`
}

type RefClaimTypes struct {
	Identifier IdentifierClaims `json:"id,omitempty"`
	Reference  ReferenceClaims  `json:"ref,omitempty"`
}

type SimpleClaimTypes struct {
	Text         TextClaims         `json:"text,omitempty"`
	String       StringClaims       `json:"string,omitempty"`
	Label        LabelClaims        `json:"label,omitempty"`
	Amount       AmountClaims       `json:"amount,omitempty"`
	AmountRange  AmountRangeClaims  `json:"amountRange,omitempty"`
	Enumeration  EnumerationClaims  `json:"enum,omitempty"`
	Relation     RelationClaims     `json:"rel,omitempty"`
	File         FileClaims         `json:"file,omitempty"`
	NoValue      NoValueClaims      `json:"none,omitempty"`
	UnknownValue UnknownValueClaims `json:"unknown,omitempty"`
}

type TimeClaimTypes struct {
	Time          TimeClaims          `json:"time,omitempty"`
	TimeRange     TimeRangeClaims     `json:"timeRange,omitempty"`
	Duration      DurationClaims      `json:"duration,omitempty"`
	DurationRange DurationRangeClaims `json:"durationRange,omitempty"`
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
	NoValueClaims       = []NoValueClaim
	UnknownValueClaims  = []UnknownValueClaim
	TimeClaims          = []TimeClaim
	TimeRangeClaims     = []TimeRangeClaim
	DurationClaims      = []DurationClaim
	DurationRangeClaims = []DurationRangeClaim
	FileClaims          = []FileClaim
	ListClaims          = []ListClaim
)

type CoreClaim struct {
	ID         Identifier  `json:"_id"`
	Confidence Confidence  `json:"confidence"`
	Meta       *MetaClaims `json:"meta,omitempty"`
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
		cc.Meta = &MetaClaims{}
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
	default:
		return errors.Errorf(`meta claim of type %T is not supported`, claim)
	}
	return nil
}

func (cc *CoreClaim) VisitMeta(visitor visitorMeta) errors.E {
	if cc.Meta == nil {
		return nil
	}

	k := 0
	for i, claim := range cc.Meta.Identifier {
		keep, err := visitor.VisitIdentifier(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.Identifier[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.Identifier) != k {
		cc.Meta.Identifier = cc.Meta.Identifier[:k]
	}

	k = 0
	for i, claim := range cc.Meta.Reference {
		keep, err := visitor.VisitReference(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.Reference[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.Reference) != k {
		cc.Meta.Reference = cc.Meta.Reference[:k]
	}

	k = 0
	for i, claim := range cc.Meta.Text {
		keep, err := visitor.VisitText(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.Text[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.Text) != k {
		cc.Meta.Text = cc.Meta.Text[:k]
	}

	k = 0
	for i, claim := range cc.Meta.String {
		keep, err := visitor.VisitString(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.String[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.String) != k {
		cc.Meta.String = cc.Meta.String[:k]
	}

	k = 0
	for i, claim := range cc.Meta.Label {
		keep, err := visitor.VisitLabel(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.Label[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.Label) != k {
		cc.Meta.Label = cc.Meta.Label[:k]
	}

	k = 0
	for i, claim := range cc.Meta.Amount {
		keep, err := visitor.VisitAmount(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.Amount[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.Amount) != k {
		cc.Meta.Amount = cc.Meta.Amount[:k]
	}

	k = 0
	for i, claim := range cc.Meta.AmountRange {
		keep, err := visitor.VisitAmountRange(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.AmountRange[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.AmountRange) != k {
		cc.Meta.AmountRange = cc.Meta.AmountRange[:k]
	}

	k = 0
	for i, claim := range cc.Meta.Enumeration {
		keep, err := visitor.VisitEnumeration(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.Enumeration[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.Enumeration) != k {
		cc.Meta.Enumeration = cc.Meta.Enumeration[:k]
	}

	k = 0
	for i, claim := range cc.Meta.Relation {
		keep, err := visitor.VisitRelation(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.Relation[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.Relation) != k {
		cc.Meta.Relation = cc.Meta.Relation[:k]
	}

	k = 0
	for i, claim := range cc.Meta.NoValue {
		keep, err := visitor.VisitNoValue(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.NoValue[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.NoValue) != k {
		cc.Meta.NoValue = cc.Meta.NoValue[:k]
	}

	k = 0
	for i, claim := range cc.Meta.UnknownValue {
		keep, err := visitor.VisitUnknownValue(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.UnknownValue[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.UnknownValue) != k {
		cc.Meta.UnknownValue = cc.Meta.UnknownValue[:k]
	}

	k = 0
	for i, claim := range cc.Meta.Time {
		keep, err := visitor.VisitTime(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.Time[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.Time) != k {
		cc.Meta.Time = cc.Meta.Time[:k]
	}

	k = 0
	for i, claim := range cc.Meta.TimeRange {
		keep, err := visitor.VisitTimeRange(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.TimeRange[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.TimeRange) != k {
		cc.Meta.TimeRange = cc.Meta.TimeRange[:k]
	}

	k = 0
	for i, claim := range cc.Meta.Duration {
		keep, err := visitor.VisitDuration(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.Duration[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.Duration) != k {
		cc.Meta.Duration = cc.Meta.Duration[:k]
	}

	k = 0
	for i, claim := range cc.Meta.DurationRange {
		keep, err := visitor.VisitDurationRange(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.DurationRange[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.DurationRange) != k {
		cc.Meta.DurationRange = cc.Meta.DurationRange[:k]
	}

	k = 0
	for i, claim := range cc.Meta.File {
		keep, err := visitor.VisitFile(&claim)
		if err != nil {
			return err
		}
		if keep == Keep {
			if i != k {
				cc.Meta.File[k] = claim
			}
			k++
		}
	}
	if len(cc.Meta.File) != k {
		cc.Meta.File = cc.Meta.File[:k]
	}

	return nil
}

func (cc *CoreClaim) GetMetaByID(id Identifier) Claim {
	v := getByIDVisitor{
		ID:     id,
		Result: nil,
	}
	_ = cc.VisitMeta(&v)
	return v.Result
}

type Confidence = Score

type Score float64

type MetaClaims struct {
	RefClaimTypes
	SimpleClaimTypes
	TimeClaimTypes
}

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

type FileClaim struct {
	CoreClaim

	Prop    DocumentReference `json:"prop"`
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Preview []string          `json:"preview,omitempty"`
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

type RelationClaim struct {
	CoreClaim

	Prop DocumentReference `json:"prop"`
	To   DocumentReference `json:"to"`
}
