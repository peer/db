package search_test

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"os"
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
		ID:          "2fjzZyP7rv8E4aHnBc6KAa",
		Name:        "type",
		Description: `Type of the document.`,
		Type:        "rel",
		Score:       0,
		RelatedDocuments: []relPropertyValue{
			{
				ID:          "JT9bhAfn5QnDzRyyLARLQn",
				Name:        "artwork",
				Description: "The document is about an artwork.",
				Score:       0,
			},
			{
				ID:          "8z5YTfJAd2c23dd5WFv4R5",
				Name:        "artist",
				Description: "The document is about an artist.",
				Score:       0,
			},
		},
	},
	{
		ID:          "KhqMjmabSREw9RdM3meEDe",
		Name:        "department",
		Description: `The department of the artwork.`,
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
		Description: `The artist of the artwork.`,
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
		Description: `The classification of the artwork.`,
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
		Description: `The medium of the artwork.`,
		Type:        "string",
		Score:       0,
	},
	{
		ID:          "19tRKrBZDkrh9PA8M8CsWZ",
		Name:        "credit",
		Description: `The credit of the artwork.`,
		Type:        "string",
		Score:       0,
	},
	{
		ID:          "FS2y5jBSy57EoHbhN3Z5Yk",
		Name:        "date acquired",
		Description: `The date acquired of the artwork.`,
		Type:        "time",
		Score:       0,
	},
	{
		ID:          "46LYApiUCkAakxrTZ82Q8Z",
		Name:        "height",
		Description: `The height of the artwork.`,
		Type:        "amount",
		Unit:        "meter",
		Score:       0,
	},
	{
		ID:          "QkCPXCeevJbB9nyi2APwBy",
		Name:        "nationality",
		Description: `The nationality of the artist.`,
		Type:        "string",
		Score:       0,
	},
	{
		ID:          "67RqCQeWbttCPHdPicN6DT",
		Name:        "gender",
		Description: `The gender of the artist.`,
		Type:        "string",
		Score:       0,
	},
	{
		ID:          "2HXMnTyFK7BCbCv6Y8231j",
		Name:        "born",
		Description: `When was the artist born.`,
		Type:        "time",
		Score:       0,
	},
	{
		ID:          "8Ls3yxCNM7a7EEEsJhNeQ6",
		Name:        "death",
		Description: `When did the artist die.`,
		Type:        "time",
		Score:       0,
	},
	{
		ID:          "4ko3ggksg89apAY8vo64VP",
		Name:        "depth",
		Description: `The depth of the artwork.`,
		Type:        "amount",
		Unit:        "meter",
		Score:       0,
	},
	{
		ID:          "HXdyya72uTpnmwscX9QpTi",
		Name:        "duration",
		Description: `The duration of the artwork.`,
		Type:        "amount",
		Unit:        "second",
		Score:       0,
	},
	{
		ID:          "K2A24W4rtqGvy1gpPpikjp",
		Name:        "diameter",
		Description: `The diameter of the artwork.`,
		Type:        "amount",
		Unit:        "meter",
		Score:       0,
	},
	{
		ID:          "8VNwPL2fRzjF1qEmv9tpud",
		Name:        "length",
		Description: `The length of the artwork.`,
		Type:        "amount",
		Unit:        "meter",
		Score:       0,
	},
	{
		ID:          "39oo9aL9YTubVnowYpqBs2",
		Name:        "weight",
		Description: `The weight of the artwork.`,
		Type:        "amount",
		Unit:        "kilogram",
		Score:       0,
	},
	{
		ID:          "VUdAU3pxVLtrHgi1yxpkqy",
		Name:        "circumference",
		Description: `The circumference of the artwork.`,
		Type:        "amount",
		Unit:        "meter",
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
	ID  string `json:"property_id"`
	Min string `json:"min"`
	Max string `json:"max"`
}

type outputFilterStructAmount struct {
	ID  string  `json:"property_id"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
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
						"type": "string",
						"description": "The search engine filters to those documents which have the property with timestamp larger or equal to the minimum. In ISO 8601 format."
					},
					"max": {
						"type": "string",
						"description": "The search engine filters to those documents which have the property with timestamp smaller or equal to the maximum. In ISO 8601 format."
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
						"type": "number",
						"description": "The search engine filters to those documents which have the property with numeric value larger or equal to the minimum."
					},
					"max": {
						"type": "number",
						"description": "The search engine filters to those documents which have the property with numeric value smaller or equal to the maximum."
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
	ID          string  `json:"document_id"`
	Name        string  `json:"document_name"`
	Description string  `json:"document_description"`
	Score       float64 `json:"relevance_score"`
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
				if z == "artworks" {
					output = append(output, "artwork")
				}
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

type textCase struct {
	Input           string
	PossibleOutputs []outputStruct
}

func TestParsePrompt(t *testing.T) {
	t.Parallel()

	tests := []textCase{
		{
			Input: "bridges",
			PossibleOutputs: []outputStruct{
				{
					Query:         "bridges",
					RelFilters:    []outputFilterStructRel{},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
			},
		},
		{
			Input: "artworks",
			PossibleOutputs: []outputStruct{
				{
					Query:         "",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         " ",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         "*",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
			},
		},
		{
			Input: `"artworks"`,
			PossibleOutputs: []outputStruct{
				{
					Query:         "",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         " ",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         "*",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
			},
		},
		{
			Input: `Find me all documents with type "artworks".`,
			PossibleOutputs: []outputStruct{
				{
					Query:         "",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         " ",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         "*",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
			},
		},
		{
			Input: "images with bridges",
			PossibleOutputs: []outputStruct{
				{
					Query:         "images with bridges",
					RelFilters:    []outputFilterStructRel{},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         "images bridges",
					RelFilters:    []outputFilterStructRel{},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         "images + bridges",
					RelFilters:    []outputFilterStructRel{},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         "images +bridges",
					RelFilters:    []outputFilterStructRel{},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         `"images" + "bridges"`,
					RelFilters:    []outputFilterStructRel{},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
			},
		},
		{
			Input: `artworks with bridges`,
			PossibleOutputs: []outputStruct{
				{
					Query:         "bridges",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         "bridge",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
				},
				{
					Query:         "bridge | bridges",
					RelFilters:    []outputFilterStructRel{{ID: "2fjzZyP7rv8E4aHnBc6KAa", DocumentIDs: []string{"JT9bhAfn5QnDzRyyLARLQn"}}},
					StringFilters: []outputFilterStructString{},
					TimeFilters:   []outputFilterStructTime{},
					AmountFilters: []outputFilterStructAmount{},
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
								if match(property.Name, input.Query) {
									propMap[property.ID] = property
								}
							}
							for _, property := range properties {
								if property.Type != "rel" {
									continue
								}
								docs := []relPropertyValue{}
								for _, doc := range property.RelatedDocuments {
									if match(doc.Name, input.Query) {
										docs = append(docs, doc)
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
					assert.Contains(t, tt.PossibleOutputs, result)

					calls, errE := x.MarshalWithoutEscapeHTML(fun.GetTextRecorder(ct).Calls())
					require.NoError(t, errE, "% -+#.1v", errE)
					out := new(bytes.Buffer)
					err := json.Indent(out, calls, "", "  ")
					require.NoError(t, err)
					t.Log(out.String())
				})
			}
		})
	}
}
