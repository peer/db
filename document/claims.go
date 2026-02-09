// Package document provides data structures and operations for PeerDB documents and their claims.
package document

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
)

// Claim is the interface for all claim types in PeerDB documents.
type Claim interface {
	ClaimsContainer

	GetConfidence() Confidence
}

var (
	_ Claim = (*IdentifierClaim)(nil)
	_ Claim = (*ReferenceClaim)(nil)
	_ Claim = (*TextClaim)(nil)
	_ Claim = (*StringClaim)(nil)
	_ Claim = (*AmountClaim)(nil)
	_ Claim = (*AmountRangeClaim)(nil)
	_ Claim = (*RelationClaim)(nil)
	_ Claim = (*FileClaim)(nil)
	_ Claim = (*NoValueClaim)(nil)
	_ Claim = (*UnknownValueClaim)(nil)
	_ Claim = (*TimeClaim)(nil)
	_ Claim = (*TimeRangeClaim)(nil)
)

// CoreDocument contains the core fields present in all PeerDB documents.
type CoreDocument struct {
	ID     identifier.Identifier `                       json:"id"`
	Score  Score                 `                       json:"score"`
	Scores Scores                `exhaustruct:"optional" json:"scores,omitempty"`
}

// GetID returns the document's identifier.
func (d CoreDocument) GetID() identifier.Identifier {
	return d.ID
}

// Mnemonic is a human-readable identifier for a document.
type Mnemonic string

// Timestamp represents a point in time, extending time.Time to support JSON marshaling with extended year format.
//
//nolint:recvcheck
type Timestamp time.Time

var timeRegex = regexp.MustCompile(`^([+-]?\d{4,})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$`)

// MarshalText implements encoding.TextMarshaler for Timestamp.
func (t Timestamp) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// MarshalJSON implements json.Marshaler for Timestamp.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	b := bytes.Buffer{}
	b.WriteString(`"`)
	b.WriteString(t.String())
	b.WriteString(`"`)
	return b.Bytes(), nil
}

// String returns the string representation of Timestamp in ISO 8601 format with extended year support.
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

// UnmarshalText implements encoding.TextUnmarshaler for Timestamp.
func (t *Timestamp) UnmarshalText(text []byte) error {
	s := string(text)
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

// UnmarshalJSON implements json.Unmarshaler for Timestamp.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var s string
	errE := x.UnmarshalWithoutUnknownFields(data, &s)
	if errE != nil {
		return errE
	}
	return t.UnmarshalText([]byte(s))
}

// TranslatableHTMLString maps language codes to HTML strings for multilingual content.
type TranslatableHTMLString map[string]string

// Scores maps score names to score values.
type Scores map[string]Score

// ClaimTypes organizes claims by their type.
type ClaimTypes struct {
	Identifier   IdentifierClaims   `exhaustruct:"optional" json:"id,omitempty"`
	Reference    ReferenceClaims    `exhaustruct:"optional" json:"ref,omitempty"`
	Text         TextClaims         `exhaustruct:"optional" json:"text,omitempty"`
	String       StringClaims       `exhaustruct:"optional" json:"string,omitempty"`
	Amount       AmountClaims       `exhaustruct:"optional" json:"amount,omitempty"`
	AmountRange  AmountRangeClaims  `exhaustruct:"optional" json:"amountRange,omitempty"`
	Relation     RelationClaims     `exhaustruct:"optional" json:"rel,omitempty"`
	File         FileClaims         `exhaustruct:"optional" json:"file,omitempty"`
	NoValue      NoValueClaims      `exhaustruct:"optional" json:"none,omitempty"`
	UnknownValue UnknownValueClaims `exhaustruct:"optional" json:"unknown,omitempty"`
	Time         TimeClaims         `exhaustruct:"optional" json:"time,omitempty"`
	TimeRange    TimeRangeClaims    `exhaustruct:"optional" json:"timeRange,omitempty"`
}

// Add adds a claim to the appropriate typed slice based on the claim's type.
func (c *ClaimTypes) Add(claim Claim) errors.E {
	switch cl := claim.(type) {
	case *IdentifierClaim:
		c.Identifier = append(c.Identifier, *cl)
	case *ReferenceClaim:
		c.Reference = append(c.Reference, *cl)
	case *TextClaim:
		c.Text = append(c.Text, *cl)
	case *StringClaim:
		c.String = append(c.String, *cl)
	case *AmountClaim:
		c.Amount = append(c.Amount, *cl)
	case *AmountRangeClaim:
		c.AmountRange = append(c.AmountRange, *cl)
	case *RelationClaim:
		c.Relation = append(c.Relation, *cl)
	case *FileClaim:
		c.File = append(c.File, *cl)
	case *NoValueClaim:
		c.NoValue = append(c.NoValue, *cl)
	case *UnknownValueClaim:
		c.UnknownValue = append(c.UnknownValue, *cl)
	case *TimeClaim:
		c.Time = append(c.Time, *cl)
	case *TimeRangeClaim:
		c.TimeRange = append(c.TimeRange, *cl)
	default:
		return errors.Errorf(`claim of type %T is not supported`, claim)
	}
	return nil
}

// Size returns the total number of claims across all types.
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

// AllClaims returns all claims as a flat slice.
func (c *ClaimTypes) AllClaims() []Claim {
	if c == nil {
		return nil
	}

	v := AllClaimsVisitor{
		Result: []Claim{},
	}
	_ = c.Visit(&v)
	return v.Result
}

type (
	// IdentifierClaims is a slice of IdentifierClaim.
	IdentifierClaims = []IdentifierClaim
	// ReferenceClaims is a slice of ReferenceClaim.
	ReferenceClaims = []ReferenceClaim
	// TextClaims is a slice of TextClaim.
	TextClaims = []TextClaim
	// StringClaims is a slice of StringClaim.
	StringClaims = []StringClaim
	// AmountClaims is a slice of AmountClaim.
	AmountClaims = []AmountClaim
	// AmountRangeClaims is a slice of AmountRangeClaim.
	AmountRangeClaims = []AmountRangeClaim
	// RelationClaims is a slice of RelationClaim.
	RelationClaims = []RelationClaim
	// FileClaims is a slice of FileClaim.
	FileClaims = []FileClaim
	// NoValueClaims is a slice of NoValueClaim.
	NoValueClaims = []NoValueClaim
	// UnknownValueClaims is a slice of UnknownValueClaim.
	UnknownValueClaims = []UnknownValueClaim
	// TimeClaims is a slice of TimeClaim.
	TimeClaims = []TimeClaim
	// TimeRangeClaims is a slice of TimeRangeClaim.
	TimeRangeClaims = []TimeRangeClaim
)

// CoreClaim contains fields common to all claim types.
//
//nolint:recvcheck
type CoreClaim struct {
	ID         identifier.Identifier `                       json:"id"`
	Confidence Confidence            `                       json:"confidence"`
	Meta       *ClaimTypes           `exhaustruct:"optional" json:"meta,omitempty"`
}

// GetID returns the claim's identifier.
func (cc CoreClaim) GetID() identifier.Identifier {
	return cc.ID
}

// GetConfidence returns the claim's confidence score.
func (cc CoreClaim) GetConfidence() Confidence {
	return cc.Confidence
}

// Visit applies a visitor to the claim's metadata claims.
func (cc *CoreClaim) Visit(visitor Visitor) errors.E {
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

// Get returns all metadata claims with the given property ID.
func (cc *CoreClaim) Get(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Keep,
		Result: []Claim{},
	}
	_ = cc.Visit(&v)
	return v.Result
}

// Remove removes and returns all metadata claims with the given property ID.
func (cc *CoreClaim) Remove(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Drop,
		Result: []Claim{},
	}
	_ = cc.Visit(&v)
	return v.Result
}

// GetByID returns the metadata claim with the given ID.
func (cc *CoreClaim) GetByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: KeepAndStop,
		Result: nil,
	}
	_ = cc.Visit(&v)
	return v.Result
}

// RemoveByID removes and returns the metadata claim with the given ID.
func (cc *CoreClaim) RemoveByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: DropAndStop,
		Result: nil,
	}
	_ = cc.Visit(&v)
	return v.Result
}

// Add adds a metadata claim to the claim.
func (cc *CoreClaim) Add(claim Claim) errors.E {
	if claimID := claim.GetID(); cc.GetByID(claimID) != nil {
		return errors.Errorf(`claim with ID "%s" already exists`, claimID)
	}
	if cc.Meta == nil {
		cc.Meta = new(ClaimTypes)
	}
	return cc.Meta.Add(claim)
}

// Size returns the number of metadata claims in the claim.
func (cc *CoreClaim) Size() int {
	return cc.Meta.Size()
}

// AllClaims returns all metadata claims as a flat slice.
func (cc *CoreClaim) AllClaims() []Claim {
	return cc.Meta.AllClaims()
}

// Confidence is an alias for Score representing the confidence level of a claim.
type Confidence = Score

// Score represents a confidence or relevance score as a float64.
type Score float64

// Reference represents a reference to another document, either by ID or as a temporary opaque reference to be resolved other.
//
// Temporary references are used to support reference cycles between documents in the same import session and allow storing
// foreign identifiers in the first pass which are then resolved to PeerDB identifiers in the second pass.
type Reference struct {
	ID *identifier.Identifier `json:"id,omitempty"`

	// Used to store temporary opaque reference before it is resolved in the second pass when importing data.
	Temporary []string `exhaustruct:"optional" json:"_temp,omitempty"` //nolint:tagliatelle
}

// IdentifierClaim represents a claim with a string identifier value.
type IdentifierClaim struct {
	CoreClaim

	Prop  Reference `json:"prop"`
	Value string    `json:"value"`
}

// ReferenceClaim represents a claim with an IRI (Internationalized Resource Identifier) value.
type ReferenceClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
	IRI  string    `json:"iri"`
}

// TextClaim represents a claim with HTML text content in multiple languages.
type TextClaim struct {
	CoreClaim

	Prop Reference              `json:"prop"`
	HTML TranslatableHTMLString `json:"html"`
}

// StringClaim represents a claim with a plain string value.
type StringClaim struct {
	CoreClaim

	Prop   Reference `json:"prop"`
	String string    `json:"string"`
}

// AmountUnit represents the unit of measurement for an amount claim.
//
//nolint:recvcheck
type AmountUnit int

const (
	// AmountUnitCustom represents a custom amount unit.
	AmountUnitCustom AmountUnit = iota
	// AmountUnitNone represents no specific unit.
	AmountUnitNone
	// AmountUnitRatio represents a dimensionless ratio unit.
	AmountUnitRatio
	// AmountUnitLitre represents the litre unit.
	AmountUnitLitre
	// AmountUnitKilogramPerKilogram represents the kilogram per kilogram ratio unit.
	AmountUnitKilogramPerKilogram
	// AmountUnitKilogram represents the kilogram mass unit.
	AmountUnitKilogram
	// AmountUnitKilogramPerCubicMetre represents the kilogram per cubic metre density unit.
	AmountUnitKilogramPerCubicMetre
	// AmountUnitMetre represents the metre length unit.
	AmountUnitMetre
	// AmountUnitSquareMetre represents the square metre area unit.
	AmountUnitSquareMetre
	// AmountUnitMetrePerSecond represents the metre per second velocity unit.
	AmountUnitMetrePerSecond
	// AmountUnitVolt represents the volt electric potential unit.
	AmountUnitVolt
	// AmountUnitWatt represents the watt power unit.
	AmountUnitWatt
	// AmountUnitPascal represents the pascal pressure unit.
	AmountUnitPascal
	// AmountUnitCoulomb represents the coulomb electric charge unit.
	AmountUnitCoulomb
	// AmountUnitJoule represents the joule energy unit.
	AmountUnitJoule
	// AmountUnitCelsius represents the Celsius temperature unit.
	AmountUnitCelsius
	// AmountUnitRadian represents the radian angle unit.
	AmountUnitRadian
	// AmountUnitHertz represents the hertz frequency unit.
	AmountUnitHertz
	// AmountUnitDollar represents the dollar currency unit.
	AmountUnitDollar
	// AmountUnitByte represents the byte data size unit.
	AmountUnitByte
	// AmountUnitPixel represents the pixel screen measurement unit.
	AmountUnitPixel
	// AmountUnitSecond represents the second time unit.
	AmountUnitSecond
	// AmountUnitDecibel represents the decibel sound intensity unit.
	AmountUnitDecibel

	// AmountUnitsTotal is the count of the number of possible amount unit values.
	AmountUnitsTotal
)

// MarshalJSON implements json.Marshaler for AmountUnit.
func (u AmountUnit) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	switch u {
	case AmountUnitCustom:
		buffer.WriteString("@")
	case AmountUnitNone:
		buffer.WriteString("1")
	case AmountUnitRatio:
		buffer.WriteString("/")
	case AmountUnitLitre:
		buffer.WriteString("l")
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
	case AmountUnitDecibel:
		buffer.WriteString("dB")
	case AmountUnitsTotal:
		fallthrough
	default:
		panic(errors.Errorf("invalid AmountUnit value: %d", u))
	}
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

// UnmarshalJSON implements json.Unmarshaler for AmountUnit.
func (u *AmountUnit) UnmarshalJSON(b []byte) error {
	var s string
	errE := x.UnmarshalWithoutUnknownFields(b, &s)
	if errE != nil {
		return errE
	}
	switch s {
	case "@":
		*u = AmountUnitCustom
	case "1":
		*u = AmountUnitNone
	case "/":
		*u = AmountUnitRatio
	case "l":
		*u = AmountUnitLitre
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
	case "dB":
		*u = AmountUnitDecibel
	default:
		return errors.Errorf("unknown amount unit: %s", s)
	}
	return nil
}

// JSONSchemaAlias returns the JSON schema alias for AmountUnit.
func (u AmountUnit) JSONSchemaAlias() any {
	return ""
}

// ValidAmountUnit checks if a given string represents a valid amount unit.
func ValidAmountUnit(unit string) bool {
	var u AmountUnit
	err := x.UnmarshalWithoutUnknownFields([]byte(`"`+unit+`"`), &u)
	return err == nil
}

// AmountClaim represents a claim with a numeric amount and unit.
type AmountClaim struct {
	CoreClaim

	Prop   Reference  `json:"prop"`
	Amount float64    `json:"amount"`
	Unit   AmountUnit `json:"unit"`
}

// AmountRangeClaim represents a claim with a numeric range (lower to upper) and unit.
type AmountRangeClaim struct {
	CoreClaim

	Prop  Reference  `json:"prop"`
	Lower float64    `json:"lower"`
	Upper float64    `json:"upper"`
	Unit  AmountUnit `json:"unit"`
}

// RelationClaim represents a claim that relates this document to another document.
type RelationClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
	To   Reference `json:"to"`
}

// FileClaim represents a claim with a file reference including media type and URL.
type FileClaim struct {
	CoreClaim

	Prop      Reference `json:"prop"`
	MediaType string    `json:"mediaType"`
	URL       string    `json:"url"`
	Preview   []string  `json:"preview,omitempty"`
}

// NoValueClaim represents a claim that explicitly states no value exists for a property.
type NoValueClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
}

// UnknownValueClaim represents a claim where the value for a property is unknown.
type UnknownValueClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
}

// TimePrecision represents the precision level of a timestamp.
type TimePrecision int

const (
	// TimePrecisionGigaYears represents a time precision of giga-years (1 billion years).
	TimePrecisionGigaYears TimePrecision = iota
	// TimePrecisionHundredMegaYears represents a time precision of 100 million years.
	TimePrecisionHundredMegaYears
	// TimePrecisionTenMegaYears represents a time precision of 10 million years.
	TimePrecisionTenMegaYears
	// TimePrecisionMegaYears represents a time precision of 1 million years (mega-years).
	TimePrecisionMegaYears
	// TimePrecisionHundredKiloYears represents a time precision of 100 thousand years.
	TimePrecisionHundredKiloYears
	// TimePrecisionTenKiloYears represents a time precision of 10 thousand years.
	TimePrecisionTenKiloYears
	// TimePrecisionKiloYears represents a time precision of 1 thousand years (kilo-years).
	TimePrecisionKiloYears
	// TimePrecisionHundredYears represents a time precision of 100 years (centuries).
	TimePrecisionHundredYears
	// TimePrecisionTenYears represents a time precision of 10 years (decades).
	TimePrecisionTenYears
	// TimePrecisionYear represents a time precision of 1 year.
	TimePrecisionYear
	// TimePrecisionMonth represents a time precision of 1 month.
	TimePrecisionMonth
	// TimePrecisionDay represents a time precision of 1 day.
	TimePrecisionDay
	// TimePrecisionHour represents a time precision of 1 hour.
	TimePrecisionHour
	// TimePrecisionMinute represents a time precision of 1 minute.
	TimePrecisionMinute
	// TimePrecisionSecond represents a time precision of 1 second.
	TimePrecisionSecond
)

// MarshalJSON implements json.Marshaler for TimePrecision.
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

// UnmarshalJSON implements json.Unmarshaler for TimePrecision.
func (p *TimePrecision) UnmarshalJSON(b []byte) error {
	var s string
	errE := x.UnmarshalWithoutUnknownFields(b, &s)
	if errE != nil {
		return errE
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

// TimeClaim represents a claim with a timestamp and precision.
type TimeClaim struct {
	CoreClaim

	Prop      Reference     `json:"prop"`
	Timestamp Timestamp     `json:"timestamp"`
	Precision TimePrecision `json:"precision"`
}

// TimeRangeClaim represents a claim with a time range (lower to upper) and precision.
type TimeRangeClaim struct {
	CoreClaim

	Prop      Reference     `json:"prop"`
	Lower     Timestamp     `json:"lower"`
	Upper     Timestamp     `json:"upper"`
	Precision TimePrecision `json:"precision"`
}
