package search

import (
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"sync"
	"time"

	servertiming "github.com/mitchellh/go-server-timing"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"
)

const (
	maxResultsCount = 1000
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
	Unit *AmountUnit           `json:"unit,omitempty"`
	Gte  *float64              `json:"gte,omitempty"`
	Lte  *float64              `json:"lte,omitempty"`
	None bool                  `json:"none,omitempty"`
}

func (f amountFilter) Valid() errors.E {
	if f.Unit == nil {
		return errors.New("unit has to be set")
	}
	if f.Gte == nil && f.Lte == nil && !f.None {
		return errors.New("gte and lte, or none has to be set")
	}
	if f.Lte != nil && f.Gte == nil {
		return errors.New("gte has to be set if lte is set")
	}
	if f.Lte == nil && f.Gte != nil {
		return errors.New("lte has to be set if gte is set")
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
	Gte  *Timestamp            `json:"gte,omitempty"`
	Lte  *Timestamp            `json:"lte,omitempty"`
	None bool                  `json:"none,omitempty"`
}

func (f timeFilter) Valid() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.None {
		return errors.New("gte and lte, or none has to be set")
	}
	if f.Lte != nil && f.Gte == nil {
		return errors.New("gte has to be set if lte is set")
	}
	if f.Lte == nil && f.Gte != nil {
		return errors.New("lte has to be set if gte is set")
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
		return errors.New("gte and lte, or none has to be set")
	}
	if f.Lte != nil && f.Gte == nil {
		return errors.New("gte has to be set if lte is set")
	}
	if f.Lte == nil && f.Gte != nil {
		return errors.New("lte has to be set if gte is set")
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
					elastic.NewTermQuery("claims.rel.prop._id", f.Rel.Prop),
				),
			)
		}
		return elastic.NewNestedQuery("claims.rel",
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.rel.prop._id", f.Rel.Prop),
				elastic.NewTermQuery("claims.rel.to._id", f.Rel.Value),
			),
		)
	}
	if f.Amount != nil {
		if f.Amount.None {
			return elastic.NewBoolQuery().MustNot(
				elastic.NewNestedQuery("claims.amount",
					elastic.NewBoolQuery().Must(
						elastic.NewTermQuery("claims.amount.prop._id", f.Amount.Prop),
						elastic.NewTermQuery("claims.amount.unit", *f.Amount.Unit),
					),
				),
			)
		}
		return elastic.NewNestedQuery("claims.amount",
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.amount.prop._id", f.Amount.Prop),
				elastic.NewTermQuery("claims.amount.unit", *f.Amount.Unit),
				elastic.NewRangeQuery("claims.amount.amount").Lte(*f.Amount.Lte).Gte(*f.Amount.Gte),
			),
		)
	}
	if f.Time != nil {
		if f.Time.None {
			return elastic.NewBoolQuery().MustNot(
				elastic.NewNestedQuery("claims.time",
					elastic.NewTermQuery("claims.time.prop._id", f.Time.Prop),
				),
			)
		}
		return elastic.NewNestedQuery("claims.time",
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.time.prop._id", f.Time.Prop),
				elastic.NewRangeQuery("claims.time.timestamp").Lte(f.Time.Lte.String()).Gte(f.Time.Gte.String()),
			),
		)
	}
	if f.Str != nil {
		if f.Str.None {
			return elastic.NewBoolQuery().MustNot(
				elastic.NewNestedQuery("claims.string",
					elastic.NewTermQuery("claims.string.prop._id", f.Str.Prop),
				),
			)
		}
		return elastic.NewNestedQuery("claims.string",
			elastic.NewBoolQuery().Must(
				elastic.NewTermQuery("claims.string.prop._id", f.Str.Prop),
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
		return elastic.NewRangeQuery("_size").Lte(*f.Size.Lte).Gte(*f.Size.Gte)
	}
	panic(errors.New("invalid filters"))
}

// searchState represents current search state.
// Search states form a tree with a link to the previous (parent) state.
type searchState struct {
	ID       identifier.Identifier  `json:"s"`
	Text     string                 `json:"q"`
	Filters  *filters               `json:"-"`
	ParentID *identifier.Identifier `json:"-"`
	RootID   identifier.Identifier  `json:"-"`
}

// Values returns search state as query string values.
func (q *searchState) Values() url.Values {
	values := url.Values{}
	values.Set("s", q.ID.String())
	values.Set("q", q.Text)
	return values
}

// ValuesWithAt returns search state as query string values, with additional "at" parameter.
func (q *searchState) ValuesWithAt(at string) url.Values {
	values := q.Values()
	if at == "" {
		return values
	}
	values.Set("at", at)
	return values
}

// TODO: Use a database instead.
var searches = sync.Map{}

// field describes a nested field for ElasticSearch to search on.
type field struct {
	Prefix string
	Field  string
}

// TODO: Return (and log) and error on invalid search requests (e.g., filters).

// makeSearch creates a new search state given optional existing state and new queries.
func makeSearch(form url.Values) *searchState {
	var parentSearchID *identifier.Identifier
	if id, errE := identifier.FromString(form.Get("s")); errE == nil {
		parentSearchID = &id
	}

	textQuery := form.Get("q")

	var fs *filters
	fsJSON := form.Get("filters")
	if fsJSON != "" {
		var f filters
		if x.UnmarshalWithoutUnknownFields([]byte(fsJSON), &f) == nil && f.Valid() == nil {
			fs = &f
		}
	}

	id := identifier.New()
	rootID := id
	if parentSearchID != nil {
		ps, ok := searches.Load(*parentSearchID)
		if ok {
			parentSearch := ps.(*searchState) //nolint:errcheck
			// There was no change.
			if parentSearch.Text == textQuery && reflect.DeepEqual(parentSearch.Filters, fs) {
				return parentSearch
			}
			rootID = parentSearch.RootID
		} else {
			// Unknown ID.
			parentSearchID = nil
		}
	}

	sh := &searchState{
		ID:       id,
		ParentID: parentSearchID,
		RootID:   rootID,
		Text:     textQuery,
		Filters:  fs,
	}
	searches.Store(sh.ID, sh)

	return sh
}

// getOrMakeSearch resolves an existing search state if possible.
// If not, it creates a new search state.
func getOrMakeSearch(form url.Values) (*searchState, bool) {
	searchID, errE := identifier.FromString(form.Get("s"))
	if errE != nil {
		return makeSearch(form), false
	}

	sh, ok := searches.Load(searchID)
	if !ok {
		return makeSearch(form), false
	}

	textQuery := form.Get("q")

	var fs *filters
	fsJSON := form.Get("filters")
	if fsJSON != "" {
		var f filters
		if x.UnmarshalWithoutUnknownFields([]byte(fsJSON), &f) == nil && f.Valid() == nil {
			fs = &f
		}
	}

	ss := sh.(*searchState) //nolint:errcheck
	// There was a change, we make current search a parent search to a new search.
	// We allow there to not be "q" or "filters" so that it is easier to use as an API.
	if (form.Has("q") && ss.Text != textQuery) || (form.Has("filters") && !reflect.DeepEqual(ss.Filters, fs)) {
		ss = &searchState{
			ID:       identifier.New(),
			ParentID: &ss.ID,
			RootID:   ss.RootID,
			Text:     textQuery,
			Filters:  fs,
		}
		searches.Store(ss.ID, ss)
		return ss, false
	}

	return ss, true
}

// getSearch resolves an existing search state if possible.
func getSearch(form url.Values) *searchState {
	searchID, errE := identifier.FromString(form.Get("s"))
	if errE != nil {
		return nil
	}
	sh, ok := searches.Load(searchID)
	if !ok {
		return nil
	}
	textQuery := form.Get("q")
	ss := sh.(*searchState) //nolint:errcheck
	// We allow there to not be "q" so that it is easier to use as an API.
	if form.Has("q") && ss.Text != textQuery {
		return nil
	}
	return ss
}

type searchResult struct {
	ID string `json:"_id"`
}

// DocumentSearch is a GET/HEAD HTTP request handler which returns HTML frontend for searching documents.
// If search state is invalid, it redirects to a valid one.
func (s *Service) DocumentSearch(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("s").Start()
	sh, ok := getOrMakeSearch(req.Form)
	m.Stop()
	if !ok {
		// Something was not OK, so we redirect to the correct URL.
		path, err := s.Reverse("DocumentSearch", nil, sh.Values())
		if err != nil {
			s.InternalServerErrorWithError(w, req, err)
			return
		}
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit ES twice?
		w.Header().Set("Location", path)
		w.WriteHeader(http.StatusSeeOther)
		return
	} else if !req.Form.Has("q") {
		// "q" is missing, so we redirect to the correct URL.
		path, err := s.Reverse("DocumentSearch", nil, sh.ValuesWithAt(req.Form.Get("at")))
		if err != nil {
			s.InternalServerErrorWithError(w, req, err)
			return
		}
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit ES twice?
		w.Header().Set("Location", path)
		w.WriteHeader(http.StatusSeeOther)
		return
	}

	s.Home(w, req, nil)
}

func (s *Service) getSearchService(req *http.Request) (*elastic.SearchService, int64, errors.E) {
	ctx := req.Context()

	site := waf.MustGetSite[*Site](ctx)

	// The fact that TrackTotalHits is set to true is important because the count is used as the
	// number of documents of the filter on the _index field.
	return s.ESClient.Search(site.Index).FetchSource(false).Preference(getHost(req.RemoteAddr)).
		Header("X-Opaque-ID", waf.MustRequestID(ctx).String()).TrackTotalHits(true).AllowPartialSearchResults(false), site.propertiesTotal, nil
}

// TODO: Determine which operator should be the default?
// TODO: Make sure right analyzers are used for all fields.
// TODO: Limit allowed syntax for simple queries (disable fuzzy matching).
func (s *Service) getSearchQuery(sh *searchState) elastic.Query { //nolint:ireturn
	boolQuery := elastic.NewBoolQuery()

	if sh.Text != "" {
		bq := elastic.NewBoolQuery()
		bq.Should(elastic.NewTermQuery("_id", sh.Text))
		for _, field := range []field{
			{"claims.id", "id"},
			{"claims.ref", "iri"},
			{"claims.text", "html.en"},
			{"claims.string", "string"},
		} {
			// TODO: Can we use simple query for keyword fields? Which analyzer is used?
			q := elastic.NewSimpleQueryStringQuery(sh.Text).Field(field.Prefix + "." + field.Field).DefaultOperator("AND")
			bq.Should(elastic.NewNestedQuery(field.Prefix, q))
		}
		boolQuery.Must(bq)
	}

	if sh.Filters != nil {
		boolQuery.Must(sh.Filters.ToQuery())
	}

	return boolQuery
}

// DocumentSearchGet is a GET/HEAD HTTP request handler and it searches ElasticSearch index using provided
// search state and returns to the client a JSON with an array of IDs of found documents. If search state is
// invalid, it returns correct query parameters as JSON. It supports compression based on accepted content
// encoding and range requests. It returns search metadata (e.g., total results) as PeerDB HTTP response headers.
func (s *Service) DocumentSearchGet(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("s").Start()
	sh, ok := getOrMakeSearch(req.Form)
	m.Stop()
	if !ok {
		// Something was not OK, so we return new query parameters.
		// TODO: Should we already do the query, to warm up ES cache?
		//       Maybe we should cache response ourselves so that we do not hit ES twice?
		s.WriteJSON(w, req, sh, nil)
		return
	}

	searchService, _, errE := s.getSearchService(req)
	if errE != nil {
		s.NotFoundWithError(w, req, errE)
		return
	}
	searchService = searchService.From(0).Size(maxResultsCount).Query(s.getSearchQuery(sh))

	m = timing.NewMetric("es").Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}
	timing.NewMetric("esi").Duration = time.Duration(res.TookInMillis) * time.Millisecond

	results := make([]searchResult, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		results[i] = searchResult{ID: hit.Id}
	}

	total := strconv.FormatInt(res.Hits.TotalHits.Value, 10)
	if res.Hits.TotalHits.Relation == "gte" {
		total += "+"
	}

	// TODO: Move this to a separate API endpoint.
	filters, err := x.MarshalWithoutEscapeHTML(sh.Filters)
	if err != nil {
		s.InternalServerErrorWithError(w, req, errors.WithStack(err))
		return
	}

	s.WriteJSON(w, req, results, map[string]interface{}{
		"total":   total,
		"query":   sh.Text,
		"filters": string(filters),
	})
}

// DocumentSearchPost is a POST HTTP request handler which stores the search state and returns
// query parameters for the GET endpoint as JSON or redirects to the GET endpoint based on search ID.
func (s *Service) DocumentSearchPost(w http.ResponseWriter, req *http.Request, _ waf.Params) {
	ctx := req.Context()
	timing := servertiming.FromContext(ctx)

	m := timing.NewMetric("s").Start()
	sh := makeSearch(req.Form)
	m.Stop()

	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit ES twice?
	s.WriteJSON(w, req, sh, nil)
}
