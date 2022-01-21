package search

import (
	"time"

	"gitlab.com/tozd/go/errors"
)

type Item struct {
	CoreDocument

	Active   *ItemClaimTypes `json:"active,omitempty"`
	Inactive *ItemClaimTypes `json:"inactive,omitempty"`
}

type Property struct {
	CoreDocument

	Mnemonic Mnemonic            `json:"mnemonic,omitempty"`
	Active   *PropertyClaimTypes `json:"active,omitempty"`
	Inactive *PropertyClaimTypes `json:"inactive,omitempty"`
}

func (p *Property) Add(claim interface{}) errors.E {
	var claimTypes *PropertyClaimTypes
	switch c := claim.(type) {
	case PropertyClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.Property = append(claimTypes.Property, c)
	case IdentifierClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.Identifier = append(claimTypes.Identifier, c)
	case ReferenceClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.Reference = append(claimTypes.Reference, c)
	case TextClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.Text = append(claimTypes.Text, c)
	case StringClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.String = append(claimTypes.String, c)
	case LabelClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.Label = append(claimTypes.Label, c)
	case AmountClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.Amount = append(claimTypes.Amount, c)
	case AmountRangeClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.AmountRange = append(claimTypes.AmountRange, c)
	case EnumerationClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.Enumeration = append(claimTypes.Enumeration, c)
	case NoValueClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.NoValue = append(claimTypes.NoValue, c)
	case UnknownValueClaim:
		if c.Confidence >= 0.0 {
			if p.Active == nil {
				p.Active = &PropertyClaimTypes{}
			}
			claimTypes = p.Active
		} else {
			if p.Inactive == nil {
				p.Inactive = &PropertyClaimTypes{}
			}
			claimTypes = p.Inactive
		}
		claimTypes.UnknownValue = append(claimTypes.UnknownValue, c)
	default:
		return errors.Errorf(`claim of type %T is not supported on a property`, claim)
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

type ItemClaimTypes struct {
	MetaClaimTypes
	SimpleClaimTypes
	TimeClaimTypes

	File FileClaims     `json:"file,omitempty"`
	List ItemListClaims `json:"list,omitempty"`
	Item ItemClaims     `json:"item,omitempty"`
}

type PropertyClaimTypes struct {
	MetaClaimTypes
	SimpleClaimTypes

	Property PropertyClaims `json:"prop,omitempty"`
}

type MetaClaimTypes struct {
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
	PropertyClaims      = []PropertyClaim
	IdentifierClaims    = []IdentifierClaim
	ReferenceClaims     = []ReferenceClaim
	TextClaims          = []TextClaim
	StringClaims        = []StringClaim
	LabelClaims         = []LabelClaim
	AmountClaims        = []AmountClaim
	AmountRangeClaims   = []AmountRangeClaim
	EnumerationClaims   = []EnumerationClaim
	NoValueClaims       = []NoValueClaim
	UnknownValueClaims  = []UnknownValueClaim
	TimeClaims          = []TimeClaim
	TimeRangeClaims     = []TimeRangeClaim
	DurationClaims      = []DurationClaim
	DurationRangeClaims = []DurationRangeClaim
	FileClaims          = []FileClaim
	ItemListClaims      = []ItemListClaim
	ItemClaims          = []ItemClaim
)

type CoreClaim struct {
	ID         Identifier  `json:"_id"`
	Confidence Confidence  `json:"confidence"`
	Meta       *MetaClaims `json:"meta,omitempty"`
}

type Confidence = Score

type Score float64

type MetaClaims struct {
	SimpleClaimTypes
	TimeClaimTypes
}

type PropertyReference = DocumentReference

type ItemReference = DocumentReference

type DocumentReference struct {
	ID     Identifier `json:"_id"`
	Name   Name       `json:"name"`
	Score  Score      `json:"score"`
	Scores Scores     `json:"scores,omitempty"`
}

type PropertyClaim struct {
	CoreClaim

	Prop  PropertyReference `json:"prop"`
	Other PropertyReference `json:"other"`
}

type IdentifierClaim struct {
	CoreClaim

	Prop       PropertyReference `json:"prop"`
	Identifier string            `json:"id"`
}

type ReferenceClaim struct {
	CoreClaim

	Prop PropertyReference `json:"prop"`
	IRI  string            `json:"iri"`
}

type TextClaim struct {
	CoreClaim

	Prop  PropertyReference       `json:"prop"`
	Plain TranslatablePlainString `json:"plain"`
	HTML  TranslatableHTMLString  `json:"html"`
}

type StringClaim struct {
	CoreClaim

	Prop   PropertyReference `json:"prop"`
	String string            `json:"string"`
}

type LabelClaim struct {
	CoreClaim

	Prop PropertyReference `json:"prop"`
}

type AmountUnit int

const (
	AmountUnitNone AmountUnit = iota
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

type AmountClaim struct {
	CoreClaim

	Prop             PropertyReference `json:"prop"`
	Amount           float64           `json:"amount"`
	UncertaintyLower float64           `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper float64           `json:"uncertaintyUpper,omitempty"`
	Unit             AmountUnit        `json:"unit"`
}

type AmountRangeClaim struct {
	CoreClaim

	Prop             PropertyReference `json:"prop"`
	Lower            float64           `json:"lower"`
	Upper            float64           `json:"upper"`
	UncertaintyLower float64           `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper float64           `json:"uncertaintyUpper,omitempty"`
	Unit             AmountUnit        `json:"unit"`
}

type EnumerationClaim struct {
	CoreClaim

	Prop PropertyReference `json:"prop"`
	Enum []string          `json:"enum"`
}

type NoValueClaim struct {
	CoreClaim

	Prop PropertyReference `json:"prop"`
}

type UnknownValueClaim struct {
	CoreClaim

	Prop PropertyReference `json:"prop"`
}

type TimePrecision int

const (
	TimePrecisionBillionYears TimePrecision = iota
	TimePrecisionHoundredMillionYears
	TimePrecisionTenMillionYears
	TimePrecisionMillionYears
	TimePrecisionHoundredMillenniums
	TimePrecisionTenMillenniums
	TimePrecisionMillennium
	TimePrecisionCentury
	TimePrecisionDecade
	TimePrecisionYear
	TimePrecisionMonth
	TimePrecisionDay
	TimePrecisionHour
	TimePrecisionMinute
	TimePrecisionSecond
)

type TimeClaim struct {
	CoreClaim

	Prop             PropertyReference `json:"prop"`
	Timestamp        Timestamp         `json:"timestamp"`
	UncertaintyLower Timestamp         `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper Timestamp         `json:"uncertaintyUpper,omitempty"`
	Precision        TimePrecision     `json:"precision,omitempty"`
}

type TimeRangeClaim struct {
	CoreClaim

	Prop             PropertyReference `json:"prop"`
	Lower            Timestamp         `json:"lower"`
	Upper            Timestamp         `json:"upper"`
	UncertaintyLower Timestamp         `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper Timestamp         `json:"uncertaintyUpper,omitempty"`
	Precision        TimePrecision     `json:"precision,omitempty"`
}

type DurationClaim struct {
	CoreClaim

	Prop             PropertyReference `json:"prop"`
	Amount           Duration          `json:"amount"`
	UncertaintyLower Duration          `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper Duration          `json:"uncertaintyUpper,omitempty"`
}

type DurationRangeClaim struct {
	CoreClaim

	Prop             PropertyReference `json:"prop"`
	Lower            Duration          `json:"lower"`
	Upper            Duration          `json:"upper"`
	UncertaintyLower Duration          `json:"uncertaintyLower,omitempty"`
	UncertaintyUpper Duration          `json:"uncertaintyUpper,omitempty"`
}

type FileClaim struct {
	CoreClaim

	Prop    PropertyReference `json:"prop"`
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Preview string            `json:"preview"`
}

type ItemListClaim struct {
	CoreClaim

	Prop     PropertyReference `json:"prop"`
	Item     ItemReference     `json:"item"`
	List     Identifier        `json:"list"`
	Order    float64           `json:"order"`
	Children []ItemListChild   `json:"children,omitempty"`
}

type ItemListChild struct {
	Prop  PropertyReference `json:"prop"`
	Child Identifier        `json:"child"`
}

type ItemClaim struct {
	CoreClaim

	Prop PropertyReference `json:"prop"`
	Item ItemReference     `json:"item"`
}
