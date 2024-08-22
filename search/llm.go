package search

import (
	"context"
	"encoding/json"
	"os"
	"slices"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/fun"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

//nolint:tagliatelle
type outputFilterStructRel struct {
	ID          string   `json:"property_id"`
	DocumentIDs []string `json:"document_ids"`
}

//nolint:tagliatelle
type outputFilterStructString struct {
	ID     string   `json:"property_id"`
	Values []string `json:"values"`
}

//nolint:tagliatelle
type outputFilterStructTime struct {
	ID  string              `json:"property_id"`
	Min *document.Timestamp `json:"min"`
	Max *document.Timestamp `json:"max"`
}

//nolint:tagliatelle
type outputFilterStructAmount struct {
	ID   string              `json:"property_id"`
	Min  *float64            `json:"min"`
	Max  *float64            `json:"max"`
	Unit document.AmountUnit `json:"unit"`
}

//nolint:tagliatelle
type outputStruct struct {
	Query         string                     `json:"query"`
	RelFilters    []outputFilterStructRel    `json:"rel_filters"`
	StringFilters []outputFilterStructString `json:"string_filters"`
	TimeFilters   []outputFilterStructTime   `json:"time_filters"`
	AmountFilters []outputFilterStructAmount `json:"amount_filters"`
}

func (s outputStruct) Filters() (*filters, errors.E) {
	f := filters{}

	for _, rel := range s.RelFilters {
		prop, errE := identifier.FromString(rel.ID)
		if errE != nil {
			return nil, errE
		}
		ids := filters{}
		for _, doc := range rel.DocumentIDs {
			d, errE := identifier.FromString(doc)
			if errE != nil {
				return nil, errE
			}
			ids.Or = append(f.Or, filters{
				Rel: &relFilter{
					Prop:  prop,
					Value: &d,
					None:  false,
				},
			})
		}
		if len(ids.Or) > 0 {
			f.And = append(f.And, ids)
		}
	}

	for _, str := range s.StringFilters {
		prop, errE := identifier.FromString(str.ID)
		if errE != nil {
			return nil, errE
		}
		values := filters{}
		for _, value := range str.Values {
			if value != "" {
				values.Or = append(values.Or, filters{
					Str: &stringFilter{
						Prop: prop,
						Str:  value,
						None: false,
					},
				})
			}
		}
		if len(values.Or) > 0 {
			f.And = append(f.And, values)
		}
	}

	for _, t := range s.TimeFilters {
		prop, errE := identifier.FromString(t.ID)
		if errE != nil {
			return nil, errE
		}
		if t.Min != nil || t.Max != nil {
			f.And = append(f.And, filters{
				Time: &timeFilter{
					Prop: prop,
					Gte:  t.Min,
					Lte:  t.Max,
					None: false,
				},
			})
		}
	}

	for _, a := range s.AmountFilters {
		prop, errE := identifier.FromString(a.ID)
		if errE != nil {
			return nil, errE
		}
		if a.Min != nil || a.Max != nil {
			f.And = append(f.And, filters{
				Amount: &amountFilter{
					Prop: prop,
					Unit: &a.Unit,
					Gte:  a.Min,
					Lte:  a.Max,
					None: false,
				},
			})
		}
	}

	if len(f.And) == 0 {
		return nil, nil
	}

	errE := f.Valid()
	if errE != nil {
		return nil, errE
	}

	return &f, nil
}

//nolint:lll,gochecknoglobals
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

const systemPrompt = `You are a parser of user queries for a search engine for documents described with property-value pairs.

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

//nolint:lll
const findPropertiesDescription = `Find properties matching the search query against their name, names of related documents, or string values. It can return multiple properties with the relevance score (higher the score, more relevant the property, related documents, or string values are to the query).`

//nolint:lll
const showResultsDescription = `Pass the search query and filters to the search engine for user to see the resulting documents. It always returns an empty string to the assistant.`

type findPropertiesInput struct {
	Query string `json:"query"`
}

//nolint:gochecknoglobals
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

//nolint:tagliatelle
type property struct {
	ID               string                `json:"property_id"`
	Name             string                `json:"property_name"`
	ExtraNames       []string              `json:"-"`
	Description      string                `json:"property_description,omitempty"`
	Type             string                `json:"property_type"`
	Unit             document.AmountUnit   `json:"unit,omitempty"`
	RelatedDocuments []relPropertyValue    `json:"related_documents,omitempty"`
	StringValues     []stringPropertyValue `json:"string_values,omitempty"`
	Score            float64               `json:"relevance_score"`
}

type findPropertiesOutput struct {
	Properties []property `json:"properties"`
	Total      int        `json:"total"`
}

//nolint:tagliatelle
type relPropertyValue struct {
	ID          string   `json:"document_id"`
	Name        string   `json:"document_name"`
	ExtraNames  []string `json:"-"`
	Description string   `json:"document_description"`
	Score       float64  `json:"relevance_score"`
}

//nolint:tagliatelle
type stringPropertyValue struct {
	Value string  `json:"value"`
	Score float64 `json:"relevance_score"`
}

func findProperties(ctx context.Context, store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes], getSearchService func() (*elastic.SearchService, int64), query string) (findPropertiesOutput, errors.E) {
	output := findPropertiesOutput{
		Properties: []property{},
		Total:      0,
	}

	bq := elastic.NewBoolQuery()
	bq.Must(documentTextSearchQuery(query))
	bq.Must(elastic.NewNestedQuery("claims.rel",
		elastic.NewBoolQuery().Must(
			elastic.NewTermQuery("claims.rel.prop.id", "CAfaL1ZZs6L4uyFdrJZ2wN"), // TYPE
			elastic.NewTermQuery("claims.rel.to.id", "E3Ua37EpwVrfxddyb9Uw64"),   // TIME_CLAIM_TYPE
		),
	))
	timePropertiesSearchSevice, _ := getSearchService()
	timePropertiesSearchSevice = timePropertiesSearchSevice.From(0).Size(MaxResultsCount).Query(bq)
	res, err := timePropertiesSearchSevice.Do(ctx)
	if err != nil {
		return output, errors.WithStack(err)
	}
	for _, hit := range res.Hits.Hits {
		data, _, _, errE := store.GetLatest(ctx, identifier.MustFromString(hit.Id))
		if errE != nil {
			return output, errE
		}

		var doc document.D
		errE = x.UnmarshalWithoutUnknownFields(data, &doc)
		if errE != nil {
			return output, errE
		}

		names := doc.Get(identifier.MustFromString("CjZig63YSyvb2KdyCL3XTg")) // NAME
		slices.SortFunc(names, func(a, b document.Claim) int {
			return int(b.GetConfidence() - a.GetConfidence())
		})

		descriptions := doc.Get(identifier.MustFromString("E7DXhBtz9UuoSG9V3uYeYF")) // DESCRIPTION
		slices.SortFunc(descriptions, func(a, b document.Claim) int {
			return int(b.GetConfidence() - a.GetConfidence())
		})

		output.Properties = append(output.Properties, property{
			ID:               doc.ID.String(),
			Name:             names[0].(*document.TextClaim).HTML["en"],
			ExtraNames:       nil,
			Description:      descriptions[0].(*document.TextClaim).HTML["en"],
			Type:             "time",
			Unit:             0,
			RelatedDocuments: nil,
			StringValues:     nil,
			Score:            0, // TODO: Set hit.Score.
		})
	}

	bq = elastic.NewBoolQuery()
	bq.Must(documentTextSearchQuery(query))
	bq.Must(elastic.NewNestedQuery("claims.rel",
		elastic.NewBoolQuery().Must(
			elastic.NewTermQuery("claims.rel.prop.id", "CAfaL1ZZs6L4uyFdrJZ2wN"), // TYPE
			elastic.NewTermQuery("claims.rel.to.id", "55JE1vpFpUvki8g2LHpN1M"),   // AMOUNT_CLAIM_TYPE
		),
	))
	amountPropertiesSearchSevice, _ := getSearchService()
	amountPropertiesSearchSevice = amountPropertiesSearchSevice.From(0).Size(MaxResultsCount).Query(bq)
	res, err = amountPropertiesSearchSevice.Do(ctx)
	if err != nil {
		return output, errors.WithStack(err)
	}
	for _, hit := range res.Hits.Hits {
		data, _, _, errE := store.GetLatest(ctx, identifier.MustFromString(hit.Id))
		if errE != nil {
			return output, errE
		}

		var doc document.D
		errE = x.UnmarshalWithoutUnknownFields(data, &doc)
		if errE != nil {
			return output, errE
		}

		names := doc.Get(identifier.MustFromString("CjZig63YSyvb2KdyCL3XTg")) // NAME
		slices.SortFunc(names, func(a, b document.Claim) int {
			return int(b.GetConfidence() - a.GetConfidence())
		})

		descriptions := doc.Get(identifier.MustFromString("E7DXhBtz9UuoSG9V3uYeYF")) // DESCRIPTION
		slices.SortFunc(descriptions, func(a, b document.Claim) int {
			return int(b.GetConfidence() - a.GetConfidence())
		})

		output.Properties = append(output.Properties, property{
			ID:               doc.ID.String(),
			Name:             names[0].(*document.TextClaim).HTML["en"],
			ExtraNames:       nil,
			Description:      descriptions[0].(*document.TextClaim).HTML["en"],
			Type:             "amount",
			Unit:             document.AmountUnitMetre, // TODO: Obtain from the document.
			RelatedDocuments: nil,
			StringValues:     nil,
			Score:            0, // TODO: Set hit.Score.
		})
	}

	// TODO: Sort by score.
	output.Total = len(output.Properties)
	return output, nil
}

func parsePrompt(ctx context.Context, store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes], getSearchService func() (*elastic.SearchService, int64), prompt string) (outputStruct, errors.E) {
	// TODO: Move out into config.
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return outputStruct{}, errors.New("ANTHROPIC_API_KEY is not available")
	}

	var result *outputStruct

	f := fun.Text[string, string]{
		Provider: &fun.AnthropicTextProvider{
			Client:      nil,
			APIKey:      os.Getenv("ANTHROPIC_API_KEY"),
			Model:       "claude-3-5-sonnet-20240620",
			Temperature: 0,
		},
		InputJSONSchema:  nil,
		OutputJSONSchema: nil,
		Prompt:           systemPrompt,
		Data:             nil,
		Tools: map[string]fun.TextTooler{
			"find_properties": &fun.TextTool[findPropertiesInput, findPropertiesOutput]{
				Description:      findPropertiesDescription,
				InputJSONSchema:  findPropertiesInputSchema,
				OutputJSONSchema: nil,
				Fun: func(ctx context.Context, input findPropertiesInput) (findPropertiesOutput, errors.E) {
					return findProperties(ctx, store, getSearchService, input.Query)
				},
			},
			"show_results": &fun.TextTool[outputStruct, string]{
				Description:      showResultsDescription,
				InputJSONSchema:  outputStructSchema,
				OutputJSONSchema: nil,
				Fun: func(_ context.Context, input outputStruct) (string, errors.E) {
					result = &input
					return "", nil
				},
			},
		},
	}

	// TODO: Do not init f every time.
	errE := f.Init(ctx)
	if errE != nil {
		return outputStruct{}, errE
	}

	_, errE = f.Call(ctx, prompt)
	if errE != nil {
		return outputStruct{}, errE
	}

	if result == nil {
		return outputStruct{}, errors.New(`"show_results" not used`)
	}

	return *result, nil
}
