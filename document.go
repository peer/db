package search

import (
	"bytes"
	"encoding/json"
	"time"

	"gitlab.com/tozd/go/errors"
)

type Document struct {
	CoreDocument

	Mnemonic Mnemonic            `json:"mnemonic,omitempty"`
	Active   *DocumentClaimTypes `json:"active,omitempty"`
	Inactive *DocumentClaimTypes `json:"inactive,omitempty"`
}

func (d *Document) Add(claim interface{}) errors.E {
	var claimTypes *DocumentClaimTypes
	switch c := claim.(type) {
	case IdentifierClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.Identifier = append(claimTypes.Identifier, c)
	case ReferenceClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.Reference = append(claimTypes.Reference, c)
	case TextClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.Text = append(claimTypes.Text, c)
	case StringClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.String = append(claimTypes.String, c)
	case LabelClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.Label = append(claimTypes.Label, c)
	case AmountClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.Amount = append(claimTypes.Amount, c)
	case AmountRangeClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.AmountRange = append(claimTypes.AmountRange, c)
	case EnumerationClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.Enumeration = append(claimTypes.Enumeration, c)
	case RelationClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.Relation = append(claimTypes.Relation, c)
	case NoValueClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.NoValue = append(claimTypes.NoValue, c)
	case UnknownValueClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.UnknownValue = append(claimTypes.UnknownValue, c)
	case TimeClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.Time = append(claimTypes.Time, c)
	case TimeRangeClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.TimeRange = append(claimTypes.TimeRange, c)
	case DurationClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.Duration = append(claimTypes.Duration, c)
	case DurationRangeClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.DurationRange = append(claimTypes.DurationRange, c)
	case FileClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.File = append(claimTypes.File, c)
	case ListClaim:
		if c.Confidence >= 0.0 {
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
		claimTypes.List = append(claimTypes.List, c)
	default:
		return errors.Errorf(`claim of type %T is not supported`, claim)
	}
	return nil
}

type CoreDocument struct {
	ID         Identifier `json:"_id"`
	Name       Name       `json:"name"`
	OtherNames OtherNames `json:"otherNames,omitempty"`
	Score      Score      `json:"score"`
	Scores     Scores     `json:"scores,omitempty"`
}

type Mnemonic string

type Identifier string

type Timestamp time.Time

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

	File FileClaims `json:"file,omitempty"`
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

	Prop  DocumentReference       `json:"prop"`
	Plain TranslatablePlainString `json:"plain"`
	HTML  TranslatableHTMLString  `json:"html"`
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
		buffer.WriteString("")
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
	case "":
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
