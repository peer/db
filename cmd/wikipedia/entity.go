package main

import (
	"bytes"
	"encoding/json"

	"gitlab.com/tozd/go/errors"
)

type EntityType int

const (
	Item EntityType = iota
	Property
)

func (t EntityType) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	switch t {
	case Item:
		buffer.WriteString("item")
	case Property:
		buffer.WriteString("property")
	}
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (t *EntityType) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	switch s {
	case "item":
		*t = Item
	case "property":
		*t = Property
	default:
		return errors.Errorf("unknown entity type: %s", s)
	}
	return nil
}

type StatementType int

const (
	Statement_ StatementType = iota
)

func (t StatementType) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	switch t {
	case Statement_:
		buffer.WriteString("statement")
	}
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (t *StatementType) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	switch s {
	case "statement":
		*t = Statement_
	default:
		return errors.Errorf("unknown statement type: %s", s)
	}
	return nil
}

type StatementRank int

const (
	Preferred StatementRank = iota
	Normal
	Deprecated
)

func (r StatementRank) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	switch r {
	case Preferred:
		buffer.WriteString("preferred")
	case Normal:
		buffer.WriteString("normal")
	case Deprecated:
		buffer.WriteString("deprecated")
	}
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (r *StatementRank) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	switch s {
	case "preferred":
		*r = Preferred
	case "normal":
		*r = Normal
	case "deprecated":
		*r = Deprecated
	default:
		return errors.Errorf("unknown statement rank: %s", s)
	}
	return nil
}

type SnakType int

const (
	Value SnakType = iota
	SomeValue
	NoValue
)

func (t SnakType) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	switch t {
	case Value:
		buffer.WriteString("value")
	case SomeValue:
		buffer.WriteString("somevalue")
	case NoValue:
		buffer.WriteString("novalue")
	}
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (t *SnakType) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	switch s {
	case "value":
		*t = Value
	case "somevalue":
		*t = SomeValue
	case "novalue":
		*t = NoValue
	default:
		return errors.Errorf("unknown snak type: %s", s)
	}
	return nil
}

type DataType int

const (
	WikiBaseItem DataType = iota
	ExternalID
	String
	Quantity
	Time
	GlobeCoordinate
	CommonsMedia
	MonolingualText
	URL
	GeoShape
	WikiBaseLexeme
	WikiBaseSense
	WikiBaseProperty
	Math
	MusicalNotation
	WikiBaseForm
	TabularData
)

func (t DataType) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	switch t {
	case WikiBaseItem:
		buffer.WriteString("wikibase-item")
	case ExternalID:
		buffer.WriteString("external-id")
	case String:
		buffer.WriteString("string")
	case Quantity:
		buffer.WriteString("quantity")
	case Time:
		buffer.WriteString("time")
	case GlobeCoordinate:
		buffer.WriteString("globe-coordinate")
	case CommonsMedia:
		buffer.WriteString("commonsMedia")
	case MonolingualText:
		buffer.WriteString("monolingualtext")
	case URL:
		buffer.WriteString("url")
	case GeoShape:
		buffer.WriteString("geo-shape")
	case WikiBaseLexeme:
		buffer.WriteString("wikibase-lexeme")
	case WikiBaseSense:
		buffer.WriteString("wikibase-sense")
	case WikiBaseProperty:
		buffer.WriteString("wikibase-property")
	case Math:
		buffer.WriteString("math")
	case MusicalNotation:
		buffer.WriteString("musical-notation")
	case WikiBaseForm:
		buffer.WriteString("wikibase-form")
	case TabularData:
		buffer.WriteString("tabular-data")
	}
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (t *DataType) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	switch s {
	case "wikibase-item":
		*t = WikiBaseItem
	case "external-id":
		*t = ExternalID
	case "string":
		*t = String
	case "quantity":
		*t = Quantity
	case "time":
		*t = Time
	case "globe-coordinate":
		*t = GlobeCoordinate
	case "commonsMedia":
		*t = CommonsMedia
	case "monolingualtext":
		*t = MonolingualText
	case "url":
		*t = URL
	case "geo-shape":
		*t = GeoShape
	case "wikibase-lexeme":
		*t = WikiBaseLexeme
	case "wikibase-sense":
		*t = WikiBaseSense
	case "wikibase-property":
		*t = WikiBaseProperty
	case "math":
		*t = Math
	case "musical-notation":
		*t = MusicalNotation
	case "wikibase-form":
		*t = WikiBaseForm
	case "tabular-data":
		*t = TabularData
	default:
		return errors.Errorf("unknown data type: %s", s)
	}
	return nil
}

type DataValueType int

const (
	StringValue DataValueType = iota
	WikiBaseEntityID
	GlobeCoordinateValue
	MonolingualTextValue
	QuantityValue
	TimeValue
)

func (t DataValueType) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	switch t {
	case StringValue:
		buffer.WriteString("string")
	case WikiBaseEntityID:
		buffer.WriteString("wikibase-entityid")
	case GlobeCoordinateValue:
		buffer.WriteString("globecoordinate")
	case MonolingualTextValue:
		buffer.WriteString("monolingualtext")
	case QuantityValue:
		buffer.WriteString("quantity")
	case TimeValue:
		buffer.WriteString("time")
	}
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (t *DataValueType) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	switch s {
	case "string":
		*t = StringValue
	case "wikibase-entityid":
		*t = WikiBaseEntityID
	case "globecoordinate":
		*t = GlobeCoordinateValue
	case "monolingualtext":
		*t = MonolingualTextValue
	case "quantity":
		*t = QuantityValue
	case "time":
		*t = TimeValue
	default:
		return errors.Errorf("unknown data value type: %s", s)
	}
	return nil
}

type DataValue struct {
	Type  DataValueType
	Value interface{} // TODO
	Error string
}

type LanguageValue struct {
	Language string
	Value    string
}

type SiteLink struct {
	Site   string
	Title  string
	Badges []string
	URL    string
}

type Snak struct {
	Hash      string
	SnakType  SnakType `json:"snaktype"`
	Property  string
	DataType  DataType  `json:"datatype"`
	DataValue DataValue `json:"datavalue"`
}

type Reference struct {
	Hash       string
	Snaks      map[string][]Snak
	SnaksOrder []string `json:"snaks-order"`
}

type Statement struct {
	ID              string
	Type            StatementType
	MainSnak        Snak `json:"mainsnak`
	Rank            StatementRank
	Qualifiers      map[string][]Snak
	QualifiersOrder []string `json:"qualifiers-order"`
	References      []Reference
}

type Entity struct {
	ID           string
	Type         EntityType
	DataType     string `json:"datatype"`
	Labels       map[string]LanguageValue
	Descriptions map[string]LanguageValue
	Aliases      map[string][]LanguageValue
	Claims       map[string][]Statement
	SiteLinks    map[string]SiteLink
	LastRevID    int64 `json:"lastrevid"`
}
