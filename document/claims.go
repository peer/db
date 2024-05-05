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

type CoreDocument struct {
	ID     identifier.Identifier `                       json:"id"`
	Score  Score                 `                       json:"score"`
	Scores Scores                `exhaustruct:"optional" json:"scores,omitempty"`
}

func (d CoreDocument) GetID() identifier.Identifier {
	return d.ID
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

func (cc *CoreClaim) Get(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Keep,
		Result: []Claim{},
	}
	_ = cc.Visit(&v)
	return v.Result
}

func (cc *CoreClaim) Remove(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Drop,
		Result: []Claim{},
	}
	_ = cc.Visit(&v)
	return v.Result
}

func (cc *CoreClaim) GetByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: KeepAndStop,
		Result: nil,
	}
	_ = cc.Visit(&v)
	return v.Result
}

func (cc *CoreClaim) RemoveByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: DropAndStop,
		Result: nil,
	}
	_ = cc.Visit(&v)
	return v.Result
}

func (cc *CoreClaim) Add(claim Claim) errors.E {
	if claimID := claim.GetID(); cc.GetByID(claimID) != nil {
		return errors.Errorf(`claim with ID "%s" already exists`, claimID)
	}
	if cc.Meta == nil {
		cc.Meta = new(ClaimTypes)
	}
	return cc.Meta.Add(claim)
}

func (cc *CoreClaim) Size() int {
	return cc.Meta.Size()
}

func (cc *CoreClaim) AllClaims() []Claim {
	v := AllClaimsVisitor{
		Result: []Claim{},
	}
	_ = cc.Visit(&v)
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

	Prop  Reference `json:"prop"`
	Value string    `json:"value"`
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

	Prop      Reference `json:"prop"`
	MediaType string    `json:"mediaType"`
	URL       string    `json:"url"`
	Preview   []string  `json:"preview,omitempty"`
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
