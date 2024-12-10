package search

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"sync"

	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/fun"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	MaxResultsCount = 1000
)

type relFilter struct {
	Prop  identifier.Identifier  `json:"prop"`
	Value *identifier.Identifier `json:"value,omitempty"`
	None  bool                   `json:"none,omitempty"`
}

func (f relFilter) Valid() errors.E {
	if f.Value == nil && !f.None {
		return errors.New("value or none has to be set")
	}
	if f.Value != nil && f.None {
		return errors.New("value and none cannot be both set")
	}
	return nil
}

type amountFilter struct {
	Prop identifier.Identifier `json:"prop"`
	Unit *document.AmountUnit  `json:"unit,omitempty"`
	Gte  *float64              `json:"gte,omitempty"`
	Lte  *float64              `json:"lte,omitempty"`
	None bool                  `json:"none,omitempty"`
}

func (f amountFilter) Valid() errors.E {
	// TODO: Why is f.Unit a pointer and can be nil at all?
	if f.Unit == nil {
		return errors.New("unit has to be set")
	}
	if f.Gte == nil && f.Lte == nil && !f.None {
		return errors.New("gte, lte, or none has to be set")
	}
	if f.Gte != nil && f.None {
		return errors.New("gte and none cannot be both set")
	}
	if f.Lte != nil && f.None {
		return errors.New("lte and none cannot be both set")
	}
	return nil
}

type timeFilter struct {
	Prop identifier.Identifier `json:"prop"`
	Gte  *document.Timestamp   `json:"gte,omitempty"`
	Lte  *document.Timestamp   `json:"lte,omitempty"`
	None bool                  `json:"none,omitempty"`
}

func (f timeFilter) Valid() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.None {
		return errors.New("gte, lte, or none has to be set")
	}
	if f.Gte != nil && f.None {
		return errors.New("gte and none cannot be both set")
	}
	if f.Lte != nil && f.None {
		return errors.New("lte and none cannot be both set")
	}
	return nil
}

type stringFilter struct {
	Prop identifier.Identifier `json:"prop"`
	Str  string                `json:"str,omitempty"`
	None bool                  `json:"none,omitempty"`
}

func (f stringFilter) Valid() errors.E {
	if f.Str == "" && !f.None {
		return errors.New("str or none has to be set")
	}
	if f.Str != "" && f.None {
		return errors.New("str and none cannot be both set")
	}
	return nil
}

type indexFilter struct {
	Str string `json:"str"`
}

func (f indexFilter) Valid() errors.E {
	if f.Str == "" {
		return errors.New("str has to be set")
	}
	return nil
}

type sizeFilter struct {
	Gte  *float64 `json:"gte,omitempty"`
	Lte  *float64 `json:"lte,omitempty"`
	None bool     `json:"none,omitempty"`
}

func (f sizeFilter) Valid() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.None {
		return errors.New("gte, lte, or none has to be set")
	}
	if f.Gte != nil && f.None {
		return errors.New("gte and none cannot be both set")
	}
	if f.Lte != nil && f.None {
		return errors.New("lte and none cannot be both set")
	}
	return nil
}

type filters struct {
	And    []filters     `json:"and,omitempty"`
	Or     []filters     `json:"or,omitempty"`
	Not    *filters      `json:"not,omitempty"`
	Rel    *relFilter    `json:"rel,omitempty"`
	Amount *amountFilter `json:"amount,omitempty"`
	Time   *timeFilter   `json:"time,omitempty"`
	Str    *stringFilter `json:"str,omitempty"`
	Index  *indexFilter  `json:"index,omitempty"`
	Size   *sizeFilter   `json:"size,omitempty"`
}

func (f filters) Valid() errors.E {
	nonEmpty := 0
	if len(f.And) > 0 {
		nonEmpty++
		for _, c := range f.And {
			err := c.Valid()
			if err != nil {
				return err
			}
		}
	}
	if len(f.Or) > 0 {
		nonEmpty++
		for _, c := range f.Or {
			err := c.Valid()
			if err != nil {
				return err
			}
		}
	}
	if f.Not != nil {
		nonEmpty++
		err := f.Not.Valid()
		if err != nil {
			return err
		}
	}
	if f.Rel != nil {
		nonEmpty++
		err := f.Rel.Valid()
		if err != nil {
			return err
		}
	}
	if f.Amount != nil {
		nonEmpty++
		err := f.Amount.Valid()
		if err != nil {
			return err
		}
	}
	if f.Time != nil {
		nonEmpty++
		err := f.Time.Valid()
		if err != nil {
			return err
		}
	}
	if f.Str != nil {
		nonEmpty++
		err := f.Str.Valid()
		if err != nil {
			return err
		}
	}
	if f.Index != nil {
		nonEmpty++
		err := f.Index.Valid()
		if err != nil {
			return err
		}
	}
	if f.Size != nil {
		nonEmpty++
		err := f.Size.Valid()
		if err != nil {
			return err
		}
	}
	if nonEmpty > 1 {
		return errors.New("only one clause can be set")
	} else if nonEmpty == 0 {
		return errors.New("no clause is set")
	}
	return nil
}

func (f filters) ToQuery() elastic.Query { //nolint:ireturn
	if len(f.And) > 0 {
		boolQuery := elastic.NewBoolQuery()
		for _, filter := range f.And {
			boolQuery.Must(filter.ToQuery())
		}
		return boolQuery
	}
	if len(f.Or) > 0 {
		boolQuery := elastic.NewBoolQuery()
		for _, filter := range f.Or {
			boolQuery.Should(filter.ToQuery())
		}
		return boolQuery
	}
	if f.Not != nil {
		boolQuery := elastic.NewBoolQuery()
		boolQuery.MustNot(f.Not.ToQuery())
		return boolQuery
	}
	if f.Rel != nil {
		if f.Rel.None {
			return elastic.NewBoolQuery().MustNot(
				elastic.NewNestedQuery("claims.rel",
					elastic.NewTermQuery("claims.rel.prop.id", f.Rel.Prop),
				),
			)
		}
		return elastic.NewNestedQuery("claims.rel",
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.rel.prop.id", f.Rel.Prop),
				elastic.NewTermQuery("claims.rel.to.id", f.Rel.Value),
			),
		)
	}
	if f.Amount != nil {
		if f.Amount.None {
			return elastic.NewBoolQuery().MustNot(
				elastic.NewNestedQuery("claims.amount",
					elastic.NewBoolQuery().Must(
						elastic.NewTermQuery("claims.amount.prop.id", f.Amount.Prop),
						elastic.NewTermQuery("claims.amount.unit", *f.Amount.Unit),
					),
				),
			)
		}
		r := elastic.NewRangeQuery("claims.amount.amount")
		if f.Amount.Lte != nil {
			r.Lte(*f.Amount.Lte)
		}
		if f.Amount.Gte != nil {
			r.Gte(*f.Amount.Gte)
		}
		return elastic.NewNestedQuery("claims.amount",
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.amount.prop.id", f.Amount.Prop),
				elastic.NewTermQuery("claims.amount.unit", *f.Amount.Unit),
				r,
			),
		)
	}
	if f.Time != nil {
		if f.Time.None {
			return elastic.NewBoolQuery().MustNot(
				elastic.NewNestedQuery("claims.time",
					elastic.NewTermQuery("claims.time.prop.id", f.Time.Prop),
				),
			)
		}
		r := elastic.NewRangeQuery("claims.time.timestamp")
		if f.Time.Lte != nil {
			r.Lte(f.Time.Lte.String())
		}
		if f.Time.Gte != nil {
			r.Gte(f.Time.Gte.String())
		}
		return elastic.NewNestedQuery("claims.time",
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.time.prop.id", f.Time.Prop),
				r,
			),
		)
	}
	if f.Str != nil {
		if f.Str.None {
			return elastic.NewBoolQuery().MustNot(
				elastic.NewNestedQuery("claims.string",
					elastic.NewTermQuery("claims.string.prop.id", f.Str.Prop),
				),
			)
		}
		return elastic.NewNestedQuery("claims.string",
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.string.prop.id", f.Str.Prop),
				elastic.NewTermQuery("claims.string.string", f.Str.Str),
			),
		)
	}
	if f.Index != nil {
		return elastic.NewTermQuery("_index", f.Index.Str)
	}
	if f.Size != nil {
		if f.Size.None {
			return elastic.NewBoolQuery().MustNot(
				elastic.NewExistsQuery("_size"),
			)
		}
		r := elastic.NewRangeQuery("_size")
		if f.Size.Lte != nil {
			r.Lte(*f.Size.Lte)
		}
		if f.Size.Gte != nil {
			r.Gte(*f.Size.Gte)
		}
		return r
	}
	panic(errors.New("invalid filters"))
}

// State represents current search state.
// Search states form a tree with a link to the previous (parent) state.
type State struct {
	ID          identifier.Identifier  `json:"s"`
	SearchQuery string                 `json:"q"`
	Prompt      string                 `json:"p,omitempty"`
	Filters     *filters               `json:"filters,omitempty"`
	ParentID    *identifier.Identifier `json:"-"`
	RootID      identifier.Identifier  `json:"-"`
	PromptDone  bool                   `json:"promptDone,omitempty"`
	PromptCalls []fun.TextRecorderCall `json:"promptCalls,omitempty"`
	PromptError bool                   `json:"promptError,omitempty"`
}

// Values returns search state as query string values.
func (s *State) Values() url.Values {
	values := url.Values{}
	if s.Prompt != "" {
		values.Set("p", s.Prompt)
	} else {
		values.Set("q", s.SearchQuery)
	}
	return values
}

// ValuesWithAt returns search state as query string values, with additional "at" parameter.
func (s *State) ValuesWithAt(at string) url.Values {
	values := s.Values()
	if at == "" {
		return values
	}
	values.Set("at", at)
	return values
}

func documentTextSearchQuery(searchQuery, defaultOperator string) elastic.Query { //nolint:ireturn
	bq := elastic.NewBoolQuery()

	if searchQuery != "" {
		bq.Should(elastic.NewTermQuery("id", searchQuery))
		for _, field := range []field{
			{"claims.id", "id"},
			{"claims.ref", "iri"},
			{"claims.text", "html.en"},
			{"claims.string", "string"},
		} {
			// TODO: Can we use simple query for keyword fields? Which analyzer is used?
			q := elastic.NewSimpleQueryStringQuery(searchQuery).Field(field.Prefix + "." + field.Field).DefaultOperator(defaultOperator)
			bq.Should(elastic.NewNestedQuery(field.Prefix, q))
		}
	}

	return bq
}

// TODO: Determine which operator should be the default?
// TODO: Make sure right analyzers are used for all fields.
// TODO: Limit allowed syntax for simple queries (disable fuzzy matching).
func (s *State) Query() elastic.Query { //nolint:ireturn
	boolQuery := elastic.NewBoolQuery()

	if s.SearchQuery != "" {
		boolQuery.Must(documentTextSearchQuery(s.SearchQuery, "AND"))
	}

	if s.Filters != nil {
		boolQuery.Must(s.Filters.ToQuery())
	}

	return boolQuery
}

func (s *State) Ready() bool {
	return s.Prompt == "" || s.PromptCalls != nil || s.PromptError
}

func (s *State) ParsePrompt(
	ctx context.Context, store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	getSearchService func() (*elastic.SearchService, int64),
) {
	ctx = fun.WithTextRecorder(ctx)

	output, errE := parsePrompt(ctx, store, getSearchService, s.Prompt)

	s.PromptDone = true
	s.PromptCalls = fun.GetTextRecorder(ctx).Calls()

	if errE != nil {
		zerolog.Ctx(ctx).Error().Err(errE).Str("prompt", s.Prompt).Interface("calls", s.PromptCalls).Msg("prompt parsing failed")
		// We reuse the prompt as the search query in this case.
		s.SearchQuery = s.Prompt
		s.PromptError = true
		searches.Store(s.ID, s)
		return
	}

	s.SearchQuery = output.Query
	s.Filters, errE = output.Filters()
	if errE != nil {
		zerolog.Ctx(ctx).Error().Err(errE).Interface("output", output).Interface("calls", s.PromptCalls).Msg("prompt filters conversion failed")
		s.PromptError = true
		searches.Store(s.ID, s)
		return
	}

	searches.Store(s.ID, s)
}

// TODO: Use a database instead.
var searches = sync.Map{} //nolint:gochecknoglobals

// field describes a nested field for ElasticSearch to search on.
type field struct {
	Prefix string
	Field  string
}

// TODO: Return (and log) and error on invalid search requests (e.g., filters).

// CreateState creates a new search state given optional existing state
// (can be an empty string) and new query/filters.
func CreateState(
	ctx context.Context, store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	getSearchService func() (*elastic.SearchService, int64), s string, searchQuery, filtersJSON string, isPrompt bool,
) *State {
	var parentSearchID *identifier.Identifier
	if id, errE := identifier.FromString(s); errE == nil {
		parentSearchID = &id
	}

	var fs *filters
	if filtersJSON != "" {
		var f filters
		if x.UnmarshalWithoutUnknownFields([]byte(filtersJSON), &f) == nil && f.Valid() == nil {
			fs = &f
		}
	}

	id := identifier.New()
	rootID := id
	if parentSearchID != nil {
		ps, ok := searches.Load(*parentSearchID)
		if ok {
			parentSearch := ps.(*State) //nolint:errcheck,forcetypeassert
			rootID = parentSearch.RootID
		} else {
			// Unknown ID.
			parentSearchID = nil
		}
	}

	if searchQuery == "" {
		// Prompt cannot be empty.
		isPrompt = false
	}

	prompt := ""
	if isPrompt {
		prompt = searchQuery
		searchQuery = ""
	}

	sh := &State{
		ID:          id,
		SearchQuery: searchQuery,
		Prompt:      prompt,
		Filters:     fs,
		ParentID:    parentSearchID,
		RootID:      rootID,
		PromptDone:  false,
		PromptCalls: nil,
		PromptError: false,
	}
	searches.Store(sh.ID, sh)

	if isPrompt {
		// We start parsing the prompt.
		// TODO: We should push parsing prompt into a proper work queue and not just make a goroutine.
		go sh.ParsePrompt(context.WithoutCancel(ctx), store, getSearchService)
	} else { //nolint:revive,staticcheck
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit store twice?
	}

	return sh
}

func createStateFromGetOrCreateState(
	ctx context.Context, store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	getSearchService func() (*elastic.SearchService, int64), s string, searchQuery, filtersJSON *string, isPrompt bool,
) (*State, bool) {
	if searchQuery == nil {
		q := ""
		searchQuery = &q
	}
	if filtersJSON == nil {
		f := ""
		filtersJSON = &f
	}
	// TODO: How to prevent that CreateState unmarshals filtersJSON again?
	// TODO: How to prevent that CreateState calls searches.Load again?
	return CreateState(ctx, store, getSearchService, s, *searchQuery, *filtersJSON, isPrompt), false
}

// GetOrCreateState resolves an existing search state if possible and validates that
// optional query/filters match those in the search state. If not, it creates a new search state.
func GetOrCreateState(
	ctx context.Context, store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	getSearchService func() (*elastic.SearchService, int64), s string, searchQuery, filtersJSON *string, isPrompt bool,
) (*State, bool) {
	searchID, errE := identifier.FromString(s)
	if errE != nil {
		return createStateFromGetOrCreateState(ctx, store, getSearchService, s, searchQuery, filtersJSON, isPrompt)
	}

	sh, ok := searches.Load(searchID)
	if !ok {
		return createStateFromGetOrCreateState(ctx, store, getSearchService, s, searchQuery, filtersJSON, isPrompt)
	}

	var fs *filters
	if filtersJSON != nil && *filtersJSON != "" {
		var f filters
		if x.UnmarshalWithoutUnknownFields([]byte(*filtersJSON), &f) == nil && f.Valid() == nil {
			fs = &f
		} else {
			// filtersJSON was invalid, so we pass nil instead.
			return createStateFromGetOrCreateState(ctx, store, getSearchService, s, searchQuery, nil, isPrompt)
		}
	}

	ss := sh.(*State) //nolint:errcheck,forcetypeassert

	if !isPrompt && searchQuery != nil && ss.SearchQuery != *searchQuery {
		return createStateFromGetOrCreateState(ctx, store, getSearchService, s, searchQuery, filtersJSON, isPrompt)
	}
	if isPrompt && searchQuery != nil && ss.Prompt != *searchQuery {
		return createStateFromGetOrCreateState(ctx, store, getSearchService, s, searchQuery, filtersJSON, isPrompt)
	}
	if filtersJSON != nil && !reflect.DeepEqual(ss.Filters, fs) {
		return createStateFromGetOrCreateState(ctx, store, getSearchService, s, searchQuery, filtersJSON, isPrompt)
	}

	return ss, true
}

// GetState resolves an existing search state if possible.
func GetState(s string) *State {
	searchID, errE := identifier.FromString(s)
	if errE != nil {
		return nil
	}
	sh, ok := searches.Load(searchID)
	if !ok {
		return nil
	}
	return sh.(*State) //nolint:forcetypeassert
}
