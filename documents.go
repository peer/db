package search

import (
	"time"
)

type Item struct {
	CoreDocument

	Active   ItemClaimTypes `json:"active"`
	Inactive ItemClaimTypes `json:"inactive"`
}

type Property struct {
	CoreDocument

	Mnemonic Mnemonic           `json:"mnemonic,omitempty"`
	Active   PropertyClaimTypes `json:"active"`
	Inactive PropertyClaimTypes `json:"inactive"`
}

type CoreDocument struct {
	ID         Identifier `json:"_id"`
	CreatedAt  Timestamp  `json:"createdAt"`
	UpdatedAt  Timestamp  `json:"updatedAt"`
	DeletedAt  *Timestamp `json:"deletedAt,omitempty"`
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
}

type MetaClaimTypes struct {
	Is         IsClaims         `json:"is,omitempty"`
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
	IsClaims            = []IsClaim
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
	ID         Identifier `json:"_id"`
	Confidence Confidence `json:"confidence"`
	Meta       MetaClaims `json:"meta,omitempty"`
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

type IsClaim struct {
	CoreClaim

	Prop PropertyReference `json:"prop"`
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
