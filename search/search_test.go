package search_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	},
	{
		ID:          "KhqMjmabSREw9RdM3meEDe",
		Name:        "department",
		Description: `The department of the artwork.`,
		Type:        "string",
		Score:       0,
	},
	{
		ID:          "J9A99CrePyKEqH6ztW1hA5",
		Name:        "by artist",
		Description: `The artist of the artwork.`,
		Type:        "rel",
		Score:       0,
	},
	{
		ID:          "UQqEUeWZmnXro2qSJYoaJZ",
		Name:        "classification",
		Description: `The classification of the artwork.`,
		Type:        "string",
		Score:       0,
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
		Score:       0,
	},
	{
		ID:          "HXdyya72uTpnmwscX9QpTi",
		Name:        "duration",
		Description: `The duration of the artwork.`,
		Type:        "amount",
		Score:       0,
	},
	{
		ID:          "K2A24W4rtqGvy1gpPpikjp",
		Name:        "diameter",
		Description: `The diameter of the artwork.`,
		Type:        "amount",
		Score:       0,
	},
	{
		ID:          "8VNwPL2fRzjF1qEmv9tpud",
		Name:        "length",
		Description: `The length of the artwork.`,
		Type:        "amount",
		Score:       0,
	},
	{
		ID:          "39oo9aL9YTubVnowYpqBs2",
		Name:        "weight",
		Description: `The weight of the artwork.`,
		Type:        "amount",
		Score:       0,
	},
	{
		ID:          "VUdAU3pxVLtrHgi1yxpkqy",
		Name:        "circumference",
		Description: `The circumference of the artwork.`,
		Type:        "amount",
		Score:       0,
	},
}

func getProperty(id string) *property {
	for _, property := range properties {
		if property.ID == id {
			return &property
		}
	}
	return nil
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
			"description": "A search query in the ElasticSearch simple_query_string syntax with all operators available and with default_operator set to AND. To not filter on text content use an empty search query."
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

- "text" properties are searched all together as text content using the search query in the ElasticSearch simple_query_string syntax with all operators available and with default_operator set to AND
- "rel" property is used for a relation to another document, based on its ID
- "string" property is used for a string value
- "time" property is used for a timestamp
- "amount" property is used for a numeric value

User might ask a question you should parse so that resulting documents answer the question,
or they might just list keywords,
or they might even provide a text content search in simple_query_string syntax already.
Determine which one it is and output a combination of the search query for "text" properties
and filters for other properties.

Use tools to determine which non-"text" properties and possible corresponding values are available to decide which filters to use.
For "rel" and "string" properties you should always check if relevant parts of the user query
match any of the possible values for them because the search query does not search over them.

All search queries (including those for tools) can be empty strings to not filter by the search query.

Unless user explicitly asks for a particular keyword (e.g., using quotation marks),
expand user query with similar keywords to find best results.
Include keywords with similar or same meaning, but different spelling.
Include keywords for related terms which could find documents of interest as well.

Use parts of the user query only once (e.g., if you use a part in a filter for a property, do not use it for another property or for the search query).

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
			"description": "A search query in the ElasticSearch simple_query_string syntax with all operators available and with default_operator set to AND. To not limit results use an empty search query."
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
	ID          string  `json:"property_id"`
	Name        string  `json:"property_name"`
	Description string  `json:"property_description"`
	Type        string  `json:"property_type"`
	Score       float64 `json:"relevance_score"`
}

type findPropertiesOutput struct {
	Properties []property `json:"properties"`
	Total      int        `json:"total"`
}

type findRelPropertyValuesInput struct {
	ID    string `json:"property_id"`
	Query string `json:"query"`
}

var findRelPropertyValuesInputSchema = []byte(`
{
	"properties": {
		"property_id": {
			"type": "string",
			"description": "ID of the property to find the possible related documents for"
		},
		"query": {
			"type": "string",
			"description": "query in the ElasticSearch simple_query_string syntax with all operators available and with default_operator set to AND"
		}
	},
	"additionalProperties": false,
	"type": "object",
	"required": [
		"property_id",
		"query"
	]
}
`)

type findRelPropertyValuesOutput struct {
	ID          string  `json:"document_id"`
	Name        string  `json:"document_name"`
	Description string  `json:"document_description"`
	Score       float64 `json:"relevance_score"`
}

type findStringPropertyValuesInput struct {
	ID    string `json:"property_id"`
	Query string `json:"query"`
}

var findStringPropertyValuesInputSchema = []byte(`
{
	"properties": {
		"property_id": {
			"type": "string",
			"description": "ID of the property to find the possible string values for"
		},
		"query": {
			"type": "string",
			"description": "query in the ElasticSearch simple_query_string syntax with all operators available and with default_operator set to AND"
		}
	},
	"additionalProperties": false,
	"type": "object",
	"required": [
		"property_id",
		"query"
	]
}
`)

type findStringPropertyValuesOutput struct {
	Value string  `json:"value"`
	Score float64 `json:"relevance_score"`
}

type findTimePropertyValuesInput struct {
	ID string `json:"property_id"`
}

var findTimePropertyValuesInputSchema = []byte(`
{
	"properties": {
		"property_id": {
			"type": "string",
			"description": "ID of the property to find the minimum and maximum possible timestamps for"
		}
	},
	"additionalProperties": false,
	"type": "object",
	"required": [
		"property_id"
	]
}
`)

type findTimePropertyValuesOutput struct {
	Min string `json:"min"`
	Max string `json:"max"`
}

type findAmountPropertyValuesInput struct {
	ID string `json:"property_id"`
}

var findAmountPropertyValuesInputSchema = []byte(`
{
	"properties": {
		"property_id": {
			"type": "string",
			"description": "ID of the property to find the minimum and maximum possible numeric values for"
		}
	},
	"additionalProperties": false,
	"type": "object",
	"required": [
		"property_id"
	]
}
`)

type findAmountPropertyValuesOutput struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
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

func TestParsePrompt(t *testing.T) {
	t.Parallel()

	p := strings.ReplaceAll(prompt, `{schema}`, string(outputStructSchema))

	tests := []fun.InputOutput[string, outputStruct]{
		// {
		// 	Input: []string{"bridges"},
		// 	Output: outputStruct{
		// 		Query: "bridges",
		// 	},
		// },
		{
			Input: []string{"artworks"},
			Output: outputStruct{
				Query: "artworks",
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
				Prompt:           p,
				Data:             nil,
				Tools: map[string]fun.TextTooler{
					"find_properties": &fun.TextTool[findPropertiesInput, findPropertiesOutput]{
						Description:      `Find non-"text" properties matching the search query. It can return multiple properties, each with their ID, name, description, type, and the relevance score (higher the score, more relevant the property is to the query).`,
						InputJSONSchema:  findPropertiesInputSchema,
						OutputJSONSchema: nil,
						Fun: func(ctx context.Context, input findPropertiesInput) (findPropertiesOutput, errors.E) {
							fmt.Println("find_properties", input)
							return findPropertiesOutput{
								Properties: properties,
								Total:      len(properties),
							}, nil
						},
					},
					"find_rel_property_values": &fun.TextTool[findRelPropertyValuesInput, []findRelPropertyValuesOutput]{
						Description:      `Find possible related documents matching the query for the property with "rel" property type. It can return multiple related documents, each with their ID, name, description, and the relevance score (higher the score, more relevant the related document is to the query).`,
						InputJSONSchema:  findRelPropertyValuesInputSchema,
						OutputJSONSchema: nil,
						Fun: func(ctx context.Context, input findRelPropertyValuesInput) ([]findRelPropertyValuesOutput, errors.E) {
							fmt.Println("find_rel_property_values", input)
							if p := getProperty(input.ID); p != nil {
								if p.Type != "rel" {
									return nil, errors.New(`property type not "rel"`)
								}
							} else {
								return nil, errors.New("property not found")
							}
							switch input.ID {
							case "2fjzZyP7rv8E4aHnBc6KAa":
								return []findRelPropertyValuesOutput{
									{
										ID:          "JT9bhAfn5QnDzRyyLARLQn",
										Name:        "artwork",
										Description: "The document describes an artwork.",
										Score:       0,
									},
								}, nil
							}
							return []findRelPropertyValuesOutput{}, nil
						},
					},
					"find_string_property_values": &fun.TextTool[findStringPropertyValuesInput, []findStringPropertyValuesOutput]{
						Description:      `Find possible string values matching the query for the property with "string" property type. It can return multiple string values, each with their value and the relevance score (higher the score, more relevant the value is to the query).`,
						InputJSONSchema:  findStringPropertyValuesInputSchema,
						OutputJSONSchema: nil,
						Fun: func(ctx context.Context, input findStringPropertyValuesInput) ([]findStringPropertyValuesOutput, errors.E) {
							fmt.Println("find_string_property_values", input)
							if p := getProperty(input.ID); p != nil {
								if p.Type != "string" {
									return nil, errors.New(`property type not "string"`)
								}
							} else {
								return nil, errors.New("property not found")
							}
							// switch input.ID {
							// case "KhqMjmabSREw9RdM3meEDe": // department
							// 	return []findStringPropertyValuesOutput{
							// 		{"Drawings & Prints", 0},
							// 		{"Photography", 0},
							// 		{"Architecture & Design", 0},
							// 		{"Painting & Sculpture", 0},
							// 		{"Media and Performance", 0},
							// 		{"Film", 0},
							// 	}, nil
							// case "UQqEUeWZmnXro2qSJYoaJZ": // classification
							// 	return []findStringPropertyValuesOutput{
							// 		{"Photograph", 0},
							// 		{"Print", 0},
							// 		{"Drawing", 0},
							// 		{"Design", 0},
							// 		{"Illustrated Book", 0},
							// 		{"Architecture", 0},
							// 		{"Painting", 0},
							// 		{"Video", 0},
							// 		{"Mies van der Rohe Archive", 0},
							// 		{"Sculpture", 0},
							// 		{"Multiple", 0},
							// 		{"Periodical", 0},
							// 		{"Installation", 0},
							// 		{"Audio", 0},
							// 		{"Ephemera", 0},
							// 		{"Film", 0},
							// 		{"Frank Lloyd Wright Archive", 0},
							// 		{"Collage", 0},
							// 		{"Performance", 0},
							// 		{"Textile", 0},
							// 	}, nil
							// }
							return []findStringPropertyValuesOutput{}, nil
						},
					},
					"find_time_property_values": &fun.TextTool[findTimePropertyValuesInput, findTimePropertyValuesOutput]{
						Description:      `Find the minimum and maximum possible timestamps for the property with "time" property type.`,
						InputJSONSchema:  findAmountPropertyValuesInputSchema,
						OutputJSONSchema: nil,
						Fun: func(ctx context.Context, input findTimePropertyValuesInput) (findTimePropertyValuesOutput, errors.E) {
							fmt.Println("find_time_property_values", input)
							if p := getProperty(input.ID); p != nil {
								if p.Type != "time" {
									return findTimePropertyValuesOutput{}, errors.New(`property type not "time"`)
								}
							} else {
								return findTimePropertyValuesOutput{}, errors.New("property not found")
							}
							switch input.ID {
							case "FS2y5jBSy57EoHbhN3Z5Yk": // date acquired
								return findTimePropertyValuesOutput{
									Min: "1929-11-19T00:00:00Z",
									Max: "2022-06-06T00:00:00Z",
								}, nil
							case "2HXMnTyFK7BCbCv6Y8231j": // born
								return findTimePropertyValuesOutput{
									Min: "1181-01-01T00:00:00Z",
									Max: "2017-01-01T00:00:00Z",
								}, nil
							case "8Ls3yxCNM7a7EEEsJhNeQ6": // death
								return findTimePropertyValuesOutput{
									Min: "1226-01-01T00:00:00Z",
									Max: "2022-01-01T00:00:00Z",
								}, nil
							}
							return findTimePropertyValuesOutput{
								Min: "",
								Max: "",
							}, nil
						},
					},
					"find_amount_property_values": &fun.TextTool[findAmountPropertyValuesInput, findAmountPropertyValuesOutput]{
						Description:      `Find the minimum and maximum possible numeric values for the property with "amount" property type.`,
						InputJSONSchema:  findAmountPropertyValuesInputSchema,
						OutputJSONSchema: nil,
						Fun: func(ctx context.Context, input findAmountPropertyValuesInput) (findAmountPropertyValuesOutput, errors.E) {
							fmt.Println("find_amount_property_values", input)
							if p := getProperty(input.ID); p != nil {
								if p.Type != "amount" {
									return findAmountPropertyValuesOutput{}, errors.New(`property type not "amount"`)
								}
							} else {
								return findAmountPropertyValuesOutput{}, errors.New("property not found")
							}
							switch input.ID {
							case "46LYApiUCkAakxrTZ82Q8Z": // height
								return findAmountPropertyValuesOutput{
									Min: 0.001588,
									Max: 91.4,
								}, nil
							case "4ko3ggksg89apAY8vo64VP": // depth
								return findAmountPropertyValuesOutput{
									Min: 0.001,
									Max: 18.085,
								}, nil
							case "HXdyya72uTpnmwscX9QpTi": // duration
								return findAmountPropertyValuesOutput{
									Min: 5,
									Max: 6283100,
								}, nil
							case "K2A24W4rtqGvy1gpPpikjp": // diameter
								return findAmountPropertyValuesOutput{
									Min: 0.00635,
									Max: 9.144,
								}, nil
							case "8VNwPL2fRzjF1qEmv9tpud": // length
								return findAmountPropertyValuesOutput{
									Min: 0.0127,
									Max: 83.211,
								}, nil
							case "39oo9aL9YTubVnowYpqBs2": // weight
								return findAmountPropertyValuesOutput{
									Min: 0.09,
									Max: 185070,
								}, nil
							case "VUdAU3pxVLtrHgi1yxpkqy": // circumference
								return findAmountPropertyValuesOutput{
									Min: 0.099,
									Max: 0.838,
								}, nil
							}
							return findAmountPropertyValuesOutput{
								Min: 0,
								Max: 0,
							}, nil
						},
					},
					"show_results": &fun.TextTool[outputStruct, string]{
						Description:      `Pass the search query and filters to the search engine for user to see the resulting documents. It always returns an empty string.`,
						InputJSONSchema:  outputStructSchema,
						OutputJSONSchema: nil,
						Fun: func(ctx context.Context, input outputStruct) (string, errors.E) {
							fmt.Printf("show_results: %+v\n", input)
							return "", nil
						},
					},
				},
			}

			ctx := zerolog.New(zerolog.NewTestWriter(t)).WithContext(context.Background())

			errE := f.Init(ctx)
			require.NoError(t, errE, "% -+#.1v", errE)

			for i, tt := range tests {
				tt := tt

				t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
					t.Parallel()

					ct := fun.WithTextRecorder(ctx)
					output, errE := f.Call(ct, tt.Input...)
					assert.NoError(t, errE, "% -+#.1v", errE)
					assert.Equal(t, tt.Output, output)

					calls, errE := x.MarshalWithoutEscapeHTML(fun.GetTextRecorder(ct).Calls())
					require.NoError(t, errE, "% -+#.1v", errE)
					out := new(bytes.Buffer)
					err := json.Indent(out, calls, "", "  ")
					require.NoError(t, err)
					fmt.Println(out.String())
				})
			}
		})
	}
}
