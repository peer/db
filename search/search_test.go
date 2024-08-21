package search_test

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/fun"
	"gitlab.com/tozd/go/x"
)

var properties = []property{
	{
		ID:          "CAfaL1ZZs6L4uyFdrJZ2wN",
		Name:        "type",
		ExtraNames:  []string{"is", "kind", "form", "category"},
		Description: `Type of a document.`,
		Type:        "rel",
		Score:       0,
		RelatedDocuments: []relPropertyValue{
			{
				ID:          "JT9bhAfn5QnDzRyyLARLQn",
				Name:        "artwork",
				ExtraNames:  []string{"artworks", "work of art", "piece of art", "art work", "art object", "art piece", "artistic work", "art", "art works", "artistic works"},
				Description: "A document is about an artwork.",
				Score:       0,
			},
			{
				ID:          "8z5YTfJAd2c23dd5WFv4R5",
				Name:        "artist",
				ExtraNames:  []string{"artists"},
				Description: "A document is about an artist.",
				Score:       0,
			},
		},
	},
	{
		ID:          "KhqMjmabSREw9RdM3meEDe",
		Name:        "department",
		ExtraNames:  []string{"division", "unit", "branch"},
		Description: `A department of an artwork.`,
		Type:        "string",
		Score:       0,
		StringValues: []stringPropertyValue{
			{"Drawings & Prints", 0},
			{"Photography", 0},
			{"Architecture & Design", 0},
			{"Painting & Sculpture", 0},
			{"Media and Performance", 0},
			{"Film", 0},
		},
	},
	{
		ID:          "J9A99CrePyKEqH6ztW1hA5",
		Name:        "by artist",
		Description: `An artist who made an artwork.`,
		Type:        "rel",
		Score:       0,
		RelatedDocuments: []relPropertyValue{
			{
				ID:          "N7uVMykiALJdHQe112DJvm",
				Name:        "Louise Bourgeois",
				Description: "",
				Score:       0,
			},
			{
				ID:          "NVtDf6dHdCvrGc4piB2EvD",
				Name:        "Eugène Atget",
				Description: "",
				Score:       0,
			},
			{
				ID:          "KMSo9B7371f3mmEuKYTgLD",
				Name:        "Unidentified photographer",
				Description: "",
				Score:       0,
			},
			{
				ID:          "CQLoGrGtDgJ4H1BEcdUU3u",
				Name:        "Ludwig Mies van der Rohe",
				Description: "",
				Score:       0,
			},
			{
				ID:          "1KAHpAFeQTBnAognyvVtLJ",
				Name:        "Pablo Picasso",
				Description: "",
				Score:       0,
			},
			{
				ID:          "RARuE6XNziq391DmMWH95d",
				Name:        "Lee Friedlander",
				Description: "",
				Score:       0,
			},
			{
				ID:          "GXeXqGqcuMD9JuywkEy4WQ",
				Name:        "August Sander",
				Description: "",
				Score:       0,
			},
			{
				ID:          "JDgtNz2pAHJjcZ8r1mxFHs",
				Name:        "Jean Dubuffet",
				Description: "",
				Score:       0,
			},
			{
				ID:          "6JsAnJAWsPiFxzSqa2D1Jf",
				Name:        "János Kender",
				Description: "",
				Score:       0,
			},
			{
				ID:          "HQGsRRwt4GutHHnkYRHxXh",
				Name:        "Harry Shunk",
				Description: "",
				Score:       0,
			},
		},
	},
	{
		ID:          "UQqEUeWZmnXro2qSJYoaJZ",
		Name:        "classification",
		ExtraNames:  []string{"classifying", "grouping", "class", "group"},
		Description: `A classification of an artwork.`,
		Type:        "string",
		Score:       0,
		StringValues: []stringPropertyValue{
			{"Photograph", 0},
			{"Print", 0},
			{"Drawing", 0},
			{"Design", 0},
			{"Illustrated Book", 0},
			{"Architecture", 0},
			{"Painting", 0},
			{"Video", 0},
			{"Mies van der Rohe Archive", 0},
			{"Sculpture", 0},
			{"Multiple", 0},
			{"Periodical", 0},
			{"Installation", 0},
			{"Audio", 0},
			{"Ephemera", 0},
			{"Film", 0},
			{"Frank Lloyd Wright Archive", 0},
			{"Collage", 0},
			{"Performance", 0},
			{"Textile", 0},
		},
	},
	{
		ID:          "Ntki6bVn3TtvHebm96jzdQ",
		Name:        "medium",
		ExtraNames:  []string{"art material", "material", "art media", "art medium", "artistic material", "artistic media", "artistic medium", "media", "medium", "art materials", "arts materials", "crafting material", "art tool", "art equipment", "art supply", "art supplies", "Art & Crafting Materials", "coloring supply", "oloring supplies"},
		Description: `A medium an artwork has been made on or with.`,
		Type:        "string",
		Score:       0,
	},
	{
		ID:          "19tRKrBZDkrh9PA8M8CsWZ",
		Name:        "credit",
		ExtraNames:  []string{"acknowledgement"},
		Description: `From where or how was an artwork acquired.`,
		Type:        "string",
		Score:       0,
	},
	{
		ID:          "FS2y5jBSy57EoHbhN3Z5Yk",
		Name:        "date acquired",
		ExtraNames:  []string{"time acquired"},
		Description: `A date when was an artwork acquired.`,
		Type:        "time",
		Score:       0,
	},
	{
		ID:          "46LYApiUCkAakxrTZ82Q8Z",
		Name:        "height",
		ExtraNames:  []string{"height difference"},
		Description: `A height of an object or a file.`,
		Type:        "amount",
		Unit:        "m",
		Score:       0,
	},
	{
		ID:          "QkCPXCeevJbB9nyi2APwBy",
		Name:        "nationality",
		ExtraNames:  []string{"citizenship"},
		Description: `A nationality of an artist.`,
		Type:        "string",
		Score:       0,
	},
	{
		ID:          "67RqCQeWbttCPHdPicN6DT",
		Name:        "gender",
		ExtraNames:  []string{"sex"},
		Description: `A gender of an artist.`,
		Type:        "string",
		Score:       0,
	},
	{
		ID:          "WmPwL6tUYkHDvfrBe1o52X",
		Name:        "date of birth",
		ExtraNames:  []string{"begin date", "birth date", "year of birth", "born", "time of birth", "DOB", "birthday", "birthdate", "birth", "b."},
		Description: `When was an artist born.`,
		Type:        "time",
		Score:       0,
	},
	{
		ID:          "P3QQ7Xssz1VTMGxiEwTpg7",
		Name:        "date of death",
		ExtraNames:  []string{"end date", "death date", "year of death", "death", "time of death", "DOD", "died on"},
		Description: `When did an artist die.`,
		Type:        "time",
		Score:       0,
	},
	{
		ID:          "4ko3ggksg89apAY8vo64VP",
		Name:        "depth",
		Description: `A depth of an object.`,
		Type:        "amount",
		Unit:        "m",
		Score:       0,
	},
	{
		ID:          "HXdyya72uTpnmwscX9QpTi",
		Name:        "duration",
		ExtraNames:  []string{"length of time", "time", "length", "elapsed time", "amount of time", "period"},
		Description: `A duration a recording or file has.`,
		Type:        "amount",
		Unit:        "s",
		Score:       0,
	},
	{
		ID:          "K2A24W4rtqGvy1gpPpikjp",
		Name:        "diameter",
		ExtraNames:  []string{"diametre"},
		Description: `A diameter of an object.`,
		Type:        "amount",
		Unit:        "m",
		Score:       0,
	},
	{
		ID:          "8VNwPL2fRzjF1qEmv9tpud",
		Name:        "length",
		Description: `A length of an object.`,
		Type:        "amount",
		Unit:        "m",
		Score:       0,
	},
	{
		ID:          "39oo9aL9YTubVnowYpqBs2",
		Name:        "weight",
		ExtraNames:  []string{"gravitational weight"},
		Description: `A weight of an object.`,
		Type:        "amount",
		Unit:        "kg",
		Score:       0,
	},
	{
		ID:          "VUdAU3pxVLtrHgi1yxpkqy",
		Name:        "circumference",
		ExtraNames:  []string{"perimeter of a circle or ellipse"},
		Description: `A circumference of an object.`,
		Type:        "amount",
		Unit:        "m",
		Score:       0,
	},
}

type outputFilterStructRel struct {
	ID          string   `json:"property_id"`
	DocumentIDs []string `json:"document_ids"`
}

type outputFilterStructString struct {
	ID     string   `json:"property_id"`
	Values []string `json:"values"`
}

type outputFilterStructTime struct {
	ID  string  `json:"property_id"`
	Min *string `json:"min"`
	Max *string `json:"max"`
}

type outputFilterStructAmount struct {
	ID   string   `json:"property_id"`
	Min  *float64 `json:"min"`
	Max  *float64 `json:"max"`
	Unit string   `json:"unit"`
}

type outputStruct struct {
	Query         string                     `json:"query"`
	RelFilters    []outputFilterStructRel    `json:"rel_filters"`
	StringFilters []outputFilterStructString `json:"string_filters"`
	TimeFilters   []outputFilterStructTime   `json:"time_filters"`
	AmountFilters []outputFilterStructAmount `json:"amount_filters"`
}

var outputStructSchema = []byte(`
{
	"title": "search_query_with_filters",
	"properties": {
		"query": {
			"type": "string",
			"description": "A search query for text content. It uses the search query syntax used by the search engine."
		},
		"rel_filters": {
			"type": "array",
			"items": {
				"properties": {
					"property_id": {
						"type": "string",
						"description": "ID of the property to filter on."
					},
					"document_ids": {
						"type": "array",
						"items": {
							"type": "string"
						},
						"description": "The search engine filters to those documents which have the property matching any of the listed related document IDs."
					}
				},
				"additionalProperties": false,
				"type": "object",
				"required": [
					"property_id",
					"document_ids"
				]
			}
		},
		"string_filters": {
			"type": "array",
			"items": {
				"properties": {
					"property_id": {
						"type": "string",
						"description": "ID of the property to filter on."
					},
					"values": {
						"type": "array",
						"items": {
							"type": "string"
						},
						"description": "The search engine filters to those documents which have the property matching any of the listed string values."
					}
				},
				"additionalProperties": false,
				"type": "object",
				"required": [
					"property_id",
					"values"
				]
			}
		},
		"time_filters": {
			"type": "array",
			"items": {
				"properties": {
					"property_id": {
						"type": "string",
						"description": "ID of the property to filter on."
					},
					"min": {
						"type": ["string", "null"],
						"description": "The search engine filters to those documents which have the property with timestamp larger or equal to the minimum. In full ISO 8601 format (with time, date, and UTC timezone Z). Use null if it should not be set."
					},
					"max": {
						"type": ["string", "null"],
						"description": "The search engine filters to those documents which have the property with timestamp smaller or equal to the maximum. In ISO 8601 format (with time, date, and UTC timezone Z). Use null if it should not be set."
					}
				},
				"additionalProperties": false,
				"type": "object",
				"required": [
					"property_id",
					"min",
					"max"
				]
			}
		},
		"amount_filters": {
			"type": "array",
			"items": {
				"properties": {
					"property_id": {
						"type": "string",
						"description": "ID of the property to filter on."
					},
					"min": {
						"type": ["number", "null"],
						"description": "The search engine filters to those documents which have the property with numeric value larger or equal to the minimum. Use null if it should not be set."
					},
					"max": {
						"type": ["number", "null"],
						"description": "The search engine filters to those documents which have the property with numeric value smaller or equal to the maximum. Use null if it should not be set."
					},
					"unit": {
						"type": "string",
						"description": "Standard unit used by the property and the unit in which minimum and maximum are expressed."
					}
				},
				"additionalProperties": false,
				"type": "object",
				"required": [
					"property_id",
					"min",
					"max",
					"unit"
				]
			}
		}
	},
	"additionalProperties": false,
	"type": "object",
	"required": [
		"query",
		"rel_filters",
		"string_filters",
		"time_filters",
		"amount_filters"
	]
}
`)

const prompt = `You are a parser of user queries for a search engine for documents described with property-value pairs.

Properties can be of five types:

- "text" properties are searched all together as text content using the search query
- "rel" property is used for a relation to another document, based on its ID
- "string" property is used for a string value
- "time" property is used for a timestamp
- "amount" property is used for a numeric value with an unit

The search query syntax used by the search engine supports the following operators:

- ` + "`+`" + ` signifies AND operation
- ` + "`|`" + ` signifies OR operation
- ` + "`-`" + ` negates a single token
- ` + "`\"`" + ` wraps a number of tokens to signify a phrase for searching
- ` + "`*`" + ` at the end of a term signifies a prefix query
- ` + "`(`" + ` and ` + "`)`" + ` signify precedence

Default operation between keywords is AND operation.
The search query can be empty to match all documents.

User might ask a question you should parse so that resulting documents answer the question,
or they might just list keywords,
or they might even use the search query syntax to provide a text content search.
Determine which one it is and output a combination of the search query for "text" properties and filters for other properties.

You MUST ALWAYS use the "find_properties" tool to determine which non-"text" properties and possible corresponding values are available to decide which filters to use.
Check if any part of the user query match any property name or any property value because the search query does not search over property names and property values.
You can use ` + "`|`" + ` operator to search for multiple values at once or you can use the tool multiple times.

The search engine finds only documents which match ALL the filters AND the search query combined, so you MUST use parts of the user query ONLY ONCE.
If you use a part in a filter, DO NOT USE it for another property or for the search query.
Prefer using filters over the search query.

Before answering, explain your reasoning step-by-step in tags.

At the end, you MUST use "show_results" tool to pass the search query and filters to the search engine and for user to see the resulting documents.
`

type findPropertiesInput struct {
	Query string `json:"query"`
}

var findPropertiesInputSchema = []byte(`
{
	"properties": {
		"query": {
			"type": "string",
			"description": "A search query. It uses the search query syntax used by the search engine."
		}
	},
	"additionalProperties": false,
	"type": "object",
	"required": [
		"query"
	]
}
`)

type property struct {
	ID               string                `json:"property_id"`
	Name             string                `json:"property_name"`
	ExtraNames       []string              `json:"-"`
	Description      string                `json:"property_description,omitempty"`
	Type             string                `json:"property_type"`
	Unit             string                `json:"unit,omitempty"`
	RelatedDocuments []relPropertyValue    `json:"related_documents,omitempty"`
	StringValues     []stringPropertyValue `json:"string_values,omitempty"`
	Score            float64               `json:"relevance_score"`
}

type findPropertiesOutput struct {
	Properties []property `json:"properties"`
	Total      int        `json:"total"`
}

type relPropertyValue struct {
	ID          string   `json:"document_id"`
	Name        string   `json:"document_name"`
	ExtraNames  []string `json:"-"`
	Description string   `json:"document_description"`
	Score       float64  `json:"relevance_score"`
}

type stringPropertyValue struct {
	Value string  `json:"value"`
	Score float64 `json:"relevance_score"`
}

var providers = []struct {
	Name     string
	Provider func(t *testing.T) fun.TextProvider
}{
	{
		"gpt-4o-mini",
		func(t *testing.T) fun.TextProvider {
			t.Helper()

			if os.Getenv("OPENAI_API_KEY") == "" {
				t.Skip("OPENAI_API_KEY is not available")
			}
			return &fun.OpenAITextProvider{
				Client:                nil,
				APIKey:                os.Getenv("OPENAI_API_KEY"),
				Model:                 "gpt-4o-mini-2024-07-18",
				MaxContextLength:      128_000,
				MaxResponseLength:     16_384,
				ForceOutputJSONSchema: false,
				Seed:                  42,
				Temperature:           0,
			}
		},
	},
	{
		"gpt-4o",
		func(t *testing.T) fun.TextProvider {
			t.Helper()

			if os.Getenv("OPENAI_API_KEY") == "" {
				t.Skip("OPENAI_API_KEY is not available")
			}
			return &fun.OpenAITextProvider{
				Client:                nil,
				APIKey:                os.Getenv("OPENAI_API_KEY"),
				Model:                 "gpt-4o-2024-08-06",
				MaxContextLength:      128_000,
				MaxResponseLength:     16_384,
				ForceOutputJSONSchema: false,
				Seed:                  42,
				Temperature:           0,
			}
		},
	},
	{
		"sonnet3",
		func(t *testing.T) fun.TextProvider {
			t.Helper()

			if os.Getenv("ANTHROPIC_API_KEY") == "" {
				t.Skip("ANTHROPIC_API_KEY is not available")
			}
			return &fun.AnthropicTextProvider{
				Client:      nil,
				APIKey:      os.Getenv("ANTHROPIC_API_KEY"),
				Model:       "claude-3-sonnet-20240229",
				Temperature: 0,
			}
		},
	},
	{
		"sonnet3.5",
		func(t *testing.T) fun.TextProvider {
			t.Helper()

			if os.Getenv("ANTHROPIC_API_KEY") == "" {
				t.Skip("ANTHROPIC_API_KEY is not available")
			}
			return &fun.AnthropicTextProvider{
				Client:      nil,
				APIKey:      os.Getenv("ANTHROPIC_API_KEY"),
				Model:       "claude-3-5-sonnet-20240620",
				Temperature: 0,
			}
		},
	},
	{
		"opus3",
		func(t *testing.T) fun.TextProvider {
			t.Helper()

			if os.Getenv("ANTHROPIC_API_KEY") == "" {
				t.Skip("ANTHROPIC_API_KEY is not available")
			}
			return &fun.AnthropicTextProvider{
				Client:      nil,
				APIKey:      os.Getenv("ANTHROPIC_API_KEY"),
				Model:       "claude-3-opus-20240229",
				Temperature: 0,
			}
		},
	},
	{
		"haiku",
		func(t *testing.T) fun.TextProvider {
			t.Helper()

			if os.Getenv("ANTHROPIC_API_KEY") == "" {
				t.Skip("ANTHROPIC_API_KEY is not available")
			}
			return &fun.AnthropicTextProvider{
				Client:      nil,
				APIKey:      os.Getenv("ANTHROPIC_API_KEY"),
				Model:       "claude-3-haiku-20240307",
				Temperature: 0,
			}
		},
	},
}

func iContains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func extractTerms(s string) []string {
	output := []string{}
	for _, x := range strings.Split(s, "|") {
		x = strings.TrimSpace(x)
		for _, y := range strings.Split(x, " OR ") {
			for _, z := range strings.Split(y, " ") {
				output = append(output, z)
			}
		}
	}
	return output
}

func match(s, query string) bool {
	for _, substr := range extractTerms(query) {
		if iContains(s, substr) {
			return true
		}
	}
	return false
}

type testOutput struct {
	KnownInvalid string
	Output       outputStruct
}

type testCase struct {
	Input           string
	PossibleOutputs []testOutput
}

func ptr[T any](x T) *T {
	return &x
}

func TestParsePrompt(t *testing.T) {
	t.Parallel()

	tests := []testCase{
		{
			Input: "bridges",
			PossibleOutputs: []testOutput{
				{
					"",
					outputStruct{
						Query:         "bridges",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"extra filters",
					outputStruct{
						Query:      "bridges",
						RelFilters: []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Architecture"},
							},
							{
								ID:     "KhqMjmabSREw9RdM3meEDe",
								Values: []string{"Architecture & Design"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"extra filters",
					outputStruct{
						Query:      "bridges",
						RelFilters: []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{
							{
								ID:     "KhqMjmabSREw9RdM3meEDe",
								Values: []string{"Architecture & Design"},
							},
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Architecture"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
			},
		},
		{
			Input: "artworks",
			PossibleOutputs: []testOutput{
				{
					"",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "*",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         "artworks",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         "artwork",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         "artwork art",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid document ID",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5Qn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid document ID, has query",
					outputStruct{
						Query:         "artworks",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5Qn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "artworks",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
			},
		},
		{
			Input: `"artworks"`,
			PossibleOutputs: []testOutput{
				{
					"",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "*",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         "artworks",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         `"artworks"`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query, invalid document ID",
					outputStruct{
						Query:         `"artworks"`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5Qn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query, invalid document ID",
					outputStruct{
						Query:         `artworks`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5Qn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid document ID",
					outputStruct{
						Query:         ``,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5Qn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         `artworks`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         `"artworks"`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
			},
		},
		{
			Input: `Find me all documents with type "artworks".`,
			PossibleOutputs: []testOutput{
				{
					"",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "*",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         "artworks",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"empty",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", Values: []string{"artwork"}}},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", Values: []string{"artworks"}}},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "artworks",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "type artworks",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid document ID",
					outputStruct{
						Query:         ``,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5Qn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
			},
		},
		{
			Input: "images with bridges",
			PossibleOutputs: []testOutput{
				{
					"",
					outputStruct{
						Query:         "images with bridges",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "images bridges",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "images + bridges",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "images +bridges",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         `"images" + "bridges"`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "bridge image",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "image bridge",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "image +bridge",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:      "bridge",
						RelFilters: []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Photograph"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:      "bridge",
						RelFilters: []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Photograph"},
							},
							{
								ID:     "KhqMjmabSREw9RdM3meEDe",
								Values: []string{"Photography"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:      "bridge",
						RelFilters: []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Painting", "Photograph", "Drawing"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:      "bridge",
						RelFilters: []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{
							{
								ID:     "KhqMjmabSREw9RdM3meEDe",
								Values: []string{"Photography"},
							},
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Photograph"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing images keyword",
					outputStruct{
						Query:         "bridges",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing images keyword",
					outputStruct{
						Query:         "+bridges",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing images keyword",
					outputStruct{
						Query:         "bridge",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid filter",
					outputStruct{
						Query:      "bridge",
						RelFilters: []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{{
							ID:     "has_image",
							Values: []string{"true"},
						}},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
			},
		},
		{
			Input: `artworks with bridges`,
			PossibleOutputs: []testOutput{
				{
					"",
					outputStruct{
						Query:         "bridges",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "bridge",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "bridge | bridges",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "bridge bridges",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "with bridges",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         "artworks bridges",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         "artworks + bridges",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "artworks bridges",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "artworks bridge",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         `artworks "with bridges"`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing bridges keyword",
					outputStruct{
						Query:         "artwork",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing bridges keyword",
					outputStruct{
						Query:         "artworks",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing bridges keyword, invalid property, using string filter",
					outputStruct{
						Query:      "*",
						RelFilters: []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{
							{
								ID:     "department",
								Values: []string{"Architecture & Design"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "bridges",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         `"artworks bridges"`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         `"artworks" + "bridges"`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         `artworks with bridges`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing bridges keyword",
					outputStruct{
						Query:         `*`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing bridges keyword",
					outputStruct{
						Query:         `"artworks"`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         `"artworks bridge"`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         `"artworks bridges"`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         `+artwork +"bridge"`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         `+"artworks" +"bridge"`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         `+"artworks" + "bridges"`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         `+artworks +bridges`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"artworks in query",
					outputStruct{
						Query:         `artworks +bridges`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         `+artworks +"bridge" | "bridges"`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         `artworks +bridges`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         `artwork +bridges`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         `artwork +bridge`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:      "bridge | bridges",
						RelFilters: []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Photograph", "Print", "Drawing", "Painting", "Sculpture"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:      "bridges",
						RelFilters: []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Photograph", "Print", "Drawing", "Painting", "Sculpture"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:      "bridge",
						RelFilters: []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Painting", "Drawing", "Photograph"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:      "bridge",
						RelFilters: []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Photograph", "Print", "Drawing", "Painting", "Sculpture", "Installation"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter",
					outputStruct{
						Query:      "bridges",
						RelFilters: []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Photograph", "Print", "Drawing", "Painting", "Sculpture", "Installation"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter, missing bridges keyword",
					outputStruct{
						Query:      "*",
						RelFilters: []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{
							{
								ID:     "UQqEUeWZmnXro2qSJYoaJZ",
								Values: []string{"Architecture & Design"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"using string filter, missing bridges keyword, invalid property",
					outputStruct{
						Query:      "*",
						RelFilters: []outputFilterStructRel{{ID: "type", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
						StringFilters: []outputFilterStructString{
							{
								ID:     "department",
								Values: []string{"Architecture & Design"},
							},
						},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid document ID",
					outputStruct{
						Query:         `bridges`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5Qn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid document ID, artworks in query",
					outputStruct{
						Query:         `artworks bridges`,
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5Qn"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
			},
		},
		{
			Input: `find me all works by Pablo Picasso`,
			PossibleOutputs: []testOutput{
				{
					"",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}},
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}},
							{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         "*",
						RelFilters:    []outputFilterStructRel{{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         "works Pablo Picasso",
						RelFilters:    []outputFilterStructRel{{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         "Pablo Picasso",
						RelFilters:    []outputFilterStructRel{{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         "picasso",
						RelFilters:    []outputFilterStructRel{{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         `"Pablo Picasso"`,
						RelFilters:    []outputFilterStructRel{{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property ID",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}, // This is artist, not artwork ID.
							{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property ID",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"1KAHpAFeQTBnAognyvVtLJ"}},
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}, // This is artist, not artwork ID.
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
			},
		},
		{
			Input: `artists born between 1950 and 2000`,
			PossibleOutputs: []testOutput{
				{
					"",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-01-01T00:00:00Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid timestamp format",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950"),
							Max: ptr("2000"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid timestamp format",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01"),
							Max: ptr("2000-12-31"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid timestamp format",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00"),
							Max: ptr("2000-12-31T23:59:59"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "WmPwL6tUYkHDvfrBe1o52X", DocumentIDs: []string{}}, // It is benign, but still wrong.
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing time filter",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing time filter",
					outputStruct{
						Query:         "1950 | 1951 | 1952 | 1953 | 1954 | 1955 | 1956 | 1957 | 1958 | 1959 | 1960 | 1961 | 1962 | 1963 | 1964 | 1965 | 1966 | 1967 | 1968 | 1969 | 1970 | 1971 | 1972 | 1973 | 1974 | 1975 | 1976 | 1977 | 1978 | 1979 | 1980 | 1981 | 1982 | 1983 | 1984 | 1985 | 1986 | 1987 | 1988 | 1989 | 1990 | 1991 | 1992 | 1993 | 1994 | 1995 | 1996 | 1997 | 1998 | 1999 | 2000",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing time filter",
					outputStruct{
						Query:         "born 195* | born 196* | born 197* | born 198* | born 199* | born 2000",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter, missing time filter",
					outputStruct{
						Query:         "artists",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter, missing time filter",
					outputStruct{
						Query:         `artists "born between" 1950 2000"`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter, missing time filter",
					outputStruct{
						Query:         `artists born between 1950 and 2000`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter, missing time filter",
					outputStruct{
						Query:         `artists "1950" "2000"`,
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter, invalid property",
					outputStruct{
						Query:         "artists",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property",
					outputStruct{
						Query: "artists",
						RelFilters: []outputFilterStructRel{
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}},
							{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"N7uVMykiALJdHQe112DJvm", "NVtDf6dHdCvrGc4piB2EvD", "KMSo9B7371f3mmEuKYTgLD", "CQLoGrGtDgJ4H1BEcdUU3u", "1KAHpAFeQTBnAognyvVtLJ", "RARuE6XNziq391DmMWH95d", "GXeXqGqcuMD9JuywkEy4WQ", "JDgtNz2pAHJjcZ8r1mxFHs", "6JsAnJAWsPiFxzSqa2D1Jf", "HQGsRRwt4GutHHnkYRHxXh"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter, invalid property",
					outputStruct{
						Query:         "artists",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "by artist",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter, missing time filter",
					outputStruct{
						Query:         "1950 1951 1952 1953 1954 1955 1956 1957 1958 1959 1960 1961 1962 1963 1964 1965 1966 1967 1968 1969 1970 1971 1972 1973 1974 1975 1976 1977 1978 1979 1980 1981 1982 1983 1984 1985 1986 1987 1988 1989 1990 1991 1992 1993 1994 1995 1996 1997 1998 1999 2000",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property",
					outputStruct{
						Query:         "artists",
						RelFilters:    []outputFilterStructRel{{ID: "by artist", DocumentIDs: []string{}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "by artist",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"N7uVMykiALJdHQe112DJvm", "NVtDf6dHdCvrGc4piB2EvD", "KMSo9B7371f3mmEuKYTgLD", "CQLoGrGtDgJ4H1BEcdUU3u", "1KAHpAFeQTBnAognyvVtLJ", "RARuE6XNziq391DmMWH95d", "GXeXqGqcuMD9JuywkEy4WQ", "JDgtNz2pAHJjcZ8r1mxFHs", "6JsAnJAWsPiFxzSqa2D1Jf", "HQGsRRwt4GutHHnkYRHxXh"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property",
					outputStruct{
						Query: "artists",
						RelFilters: []outputFilterStructRel{
							{ID: "J9A99CrePyKEqH6ztW1hA5", DocumentIDs: []string{"N7uVMykiALJdHQe112DJvm", "NVtDf6dHdCvrGc4piB2EvD", "KMSo9B7371f3mmEuKYTgLD", "CQLoGrGtDgJ4H1BEcdUU3u", "1KAHpAFeQTBnAognyvVtLJ", "RARuE6XNziq391DmMWH95d", "GXeXqGqcuMD9JuywkEy4WQ", "JDgtNz2pAHJjcZ8r1mxFHs", "6JsAnJAWsPiFxzSqa2D1Jf", "HQGsRRwt4GutHHnkYRHxXh"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter, invalid timestamp format",
					outputStruct{
						Query:         "artists",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950"),
							Max: ptr("2000"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter, has query",
					outputStruct{
						Query:         "artists",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing document ID",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{""}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query, invalid timestamp format",
					outputStruct{
						Query:         "born:1950-01-01..2000-12-31",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01"),
							Max: ptr("2000-12-31"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query:         "born:1950-01-01..2000-12-31",
						RelFilters:    []outputFilterStructRel{{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"8z5YTfJAd2c23dd5WFv4R5"}}},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "WmPwL6tUYkHDvfrBe1o52X",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "by artist", DocumentIDs: []string{"RARuE6XNziq391DmMWH95d", "GXeXqGqcuMD9JuywkEy4WQ"}},
							{ID: "date of birth", DocumentIDs: []string{"1950-01-01T00:00:00Z", "2000-12-31T23:59:59Z"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "date of birth",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "by artist", DocumentIDs: []string{"RARuE6XNziq391DmMWH95d", "GXeXqGqcuMD9JuywkEy4WQ"}},
							{ID: "date of birth", DocumentIDs: []string{"WmPwL6tUYkHDvfrBe1o52X"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "date of birth",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid property, has query",
					outputStruct{
						Query: "artists",
						RelFilters: []outputFilterStructRel{
							{ID: "by artist", DocumentIDs: []string{"RARuE6XNziq391DmMWH95d", "GXeXqGqcuMD9JuywkEy4WQ"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "date of birth",
							Min: ptr("1950-01-01T00:00:00Z"),
							Max: ptr("2000-12-31T23:59:59Z"),
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
			},
		},
		{
			Input: `artworks acquired after 17. 3. 1999`,
			PossibleOutputs: []testOutput{
				{
					"",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17T00:00:00Z"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"",
					outputStruct{
						Query: " ",
						RelFilters: []outputFilterStructRel{
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17T00:00:00Z"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query: "artwork",
						RelFilters: []outputFilterStructRel{
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17T00:00:00Z"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"has query",
					outputStruct{
						Query: "artworks",
						RelFilters: []outputFilterStructRel{
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17T00:00:00Z"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid timestamp format",
					outputStruct{
						Query: "",
						RelFilters: []outputFilterStructRel{
							{ID: "CAfaL1ZZs6L4uyFdrJZ2wN", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}},
						},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid timestamp format",
					outputStruct{
						Query:         "artworks",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid timestamp format",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"invalid timestamp format",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-18"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "artwork",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17T00:00:00Z"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17T00:00:00Z"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
				{
					"missing type filter",
					outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters: []outputFilterStructTime{{
							ID:  "FS2y5jBSy57EoHbhN3Z5Yk",
							Min: ptr("1999-03-17T00:00:00Z"),
							Max: nil,
						}},
						AmountFilters: []outputFilterStructAmount{},
					},
				},
			},
		},
		{
			Input: `all objects with diameter larger than 1 cm`,
			PossibleOutputs: []testOutput{
				{
					KnownInvalid: "",
					Output: outputStruct{
						Query:         "",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{
							{
								ID:   "K2A24W4rtqGvy1gpPpikjp",
								Min:  ptr(0.01),
								Max:  nil,
								Unit: "m",
							},
						},
					},
				},
				{
					KnownInvalid: "",
					Output: outputStruct{
						Query:         " ",
						RelFilters:    []outputFilterStructRel{},
						StringFilters: []outputFilterStructString{},
						TimeFilters:   []outputFilterStructTime{},
						AmountFilters: []outputFilterStructAmount{
							{
								ID:   "K2A24W4rtqGvy1gpPpikjp",
								Min:  ptr(0.01),
								Max:  nil,
								Unit: "m",
							},
						},
					},
				},
			},
		},
	}

	for _, provider := range providers {
		provider := provider

		t.Run(provider.Name, func(t *testing.T) {
			t.Parallel()

			f := fun.Text[string, string]{
				Provider:         provider.Provider(t),
				InputJSONSchema:  nil,
				OutputJSONSchema: nil,
				Prompt:           prompt,
				Data:             nil,
				Tools: map[string]fun.TextTooler{
					"find_properties": &fun.TextTool[findPropertiesInput, findPropertiesOutput]{
						Description:      `Find properties matching the search query against their name, names of related documents, or string values. It can return multiple properties with the relevance score (higher the score, more relevant the property, related documents, or string values are to the query).`,
						InputJSONSchema:  findPropertiesInputSchema,
						OutputJSONSchema: nil,
						Fun: func(ctx context.Context, input findPropertiesInput) (findPropertiesOutput, errors.E) {
							propMap := map[string]property{}
							for _, property := range properties {
								if match(property.Name, input.Query) || match(property.Description, input.Query) {
									propMap[property.ID] = property
								} else {
									for _, extraName := range property.ExtraNames {
										if match(extraName, input.Query) {
											propMap[property.ID] = property
											break
										}
									}
								}
							}
							for _, property := range properties {
								if property.Type != "rel" {
									continue
								}
								docs := []relPropertyValue{}
								for _, doc := range property.RelatedDocuments {
									if match(doc.Name, input.Query) || match(property.Description, input.Query) {
										docs = append(docs, doc)
									} else {
										for _, extraName := range doc.ExtraNames {
											if match(extraName, input.Query) {
												docs = append(docs, doc)
												break
											}
										}
									}
								}
								if len(docs) > 0 {
									// Make a copy.
									p := property
									p.RelatedDocuments = docs
									if _, ok := propMap[p.ID]; !ok {
										propMap[p.ID] = p
									}
								}
							}
							for _, property := range properties {
								if property.Type != "string" {
									continue
								}
								values := []stringPropertyValue{}
								for _, value := range property.StringValues {
									if match(value.Value, input.Query) {
										values = append(values, value)
									}
								}
								if len(values) > 0 {
									// Make a copy.
									p := property
									p.StringValues = values
									if _, ok := propMap[p.ID]; !ok {
										propMap[p.ID] = p
									}
								}
							}
							props := []property{}
							for _, p := range propMap {
								props = append(props, p)
							}
							slices.SortFunc(props, func(a, b property) int {
								return cmp.Compare(a.ID, b.ID)
							})
							return findPropertiesOutput{
								Properties: props,
								Total:      len(props),
							}, nil
						},
					},
					"show_results": &fun.TextTool[outputStruct, string]{
						Description:      `Pass the search query and filters to the search engine for user to see the resulting documents. It always returns an empty string to the assistant.`,
						InputJSONSchema:  outputStructSchema,
						OutputJSONSchema: nil,
						Fun: func(ctx context.Context, input outputStruct) (string, errors.E) {
							*ctx.Value("result").(*outputStruct) = input
							return "", nil
						},
					},
				},
			}

			ctx := zerolog.New(zerolog.NewTestWriter(t)).WithContext(context.Background())

			errE := f.Init(ctx)
			require.NoError(t, errE, "% -+#.1v", errE)

			for _, tt := range tests {
				tt := tt

				t.Run(tt.Input, func(t *testing.T) {
					t.Parallel()

					var result outputStruct

					ct := fun.WithTextRecorder(ctx)
					ct = context.WithValue(ct, "result", &result)
					_, errE := f.Call(ct, tt.Input)
					assert.NoError(t, errE, "% -+#.1v", errE)

					calls, errE := x.MarshalWithoutEscapeHTML(fun.GetTextRecorder(ct).Calls())
					require.NoError(t, errE, "% -+#.1v", errE)
					out := new(bytes.Buffer)
					err := json.Indent(out, calls, "", "  ")
					require.NoError(t, err)

					foundOutput := false
					for _, output := range tt.PossibleOutputs {
						if reflect.DeepEqual(output.Output, result) {
							if output.KnownInvalid != "" {
								t.Skipf("known invalid: %s", output.KnownInvalid)
							}
							foundOutput = true
							break
						}
					}
					if !foundOutput {
						assert.Fail(t, out.String())
					}
				})
			}
		})
	}
}
