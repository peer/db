package document

import (
	"bytes"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

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
		errE := errors.New("invalid AmountUnit value")
		errors.Details(errE)["value"] = u
		panic(errE)
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
		errE := errors.New("unknown amount unit")
		errors.Details(errE)["value"] = s
		return errE
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
