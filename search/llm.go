package search

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
	ID  string  `json:"property_id"`
	Min *string `json:"min"`
	Max *string `json:"max"`
}

//nolint:tagliatelle
type outputFilterStructAmount struct {
	ID   string   `json:"property_id"`
	Min  *float64 `json:"min"`
	Max  *float64 `json:"max"`
	Unit string   `json:"unit"`
}

//nolint:tagliatelle
type outputStruct struct {
	Query         string                     `json:"query"`
	RelFilters    []outputFilterStructRel    `json:"rel_filters"`
	StringFilters []outputFilterStructString `json:"string_filters"`
	TimeFilters   []outputFilterStructTime   `json:"time_filters"`
	AmountFilters []outputFilterStructAmount `json:"amount_filters"`
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
	Unit             string                `json:"unit,omitempty"`
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
