package search

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/document"
	internal "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	// MaxResultsCount is the maximum number of search results that can be returned.
	MaxResultsCount = 1000
)

// ViewType represents the type of search view.
type ViewType string

// ViewType values.
const (
	ViewFeed  ViewType = "feed"
	ViewTable ViewType = "table"
)

// RelFilter represents a filter for relation claims.
type RelFilter struct {
	Prop  identifier.Identifier  `json:"prop"`
	Value *identifier.Identifier `json:"value,omitempty"`
	None  bool                   `json:"none,omitempty"`
}

// Valid validates the RelFilter to ensure it has a valid configuration.
func (f RelFilter) Valid() errors.E {
	if f.Value == nil && !f.None {
		return errors.New("value or none has to be set")
	}
	if f.Value != nil && f.None {
		return errors.New("value and none cannot be both set")
	}
	return nil
}

// AmountFilter represents a filter for amount claims.
type AmountFilter struct {
	Prop identifier.Identifier `json:"prop"`
	Unit *document.AmountUnit  `json:"unit,omitempty"`
	Gte  *float64              `json:"gte,omitempty"`
	Lte  *float64              `json:"lte,omitempty"`
	None bool                  `json:"none,omitempty"`
}

// Valid validates the AmountFilter to ensure it has a valid configuration.
func (f AmountFilter) Valid() errors.E {
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

// TimeFilter represents a filter for time claims.
type TimeFilter struct {
	Prop identifier.Identifier `json:"prop"`
	Gte  *document.Timestamp   `json:"gte,omitempty"`
	Lte  *document.Timestamp   `json:"lte,omitempty"`
	None bool                  `json:"none,omitempty"`
}

// Valid validates the TimeFilter to ensure it has a valid configuration.
func (f TimeFilter) Valid() errors.E {
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

// StringFilter represents a filter for string claims.
type StringFilter struct {
	Prop identifier.Identifier `json:"prop"`
	Str  string                `json:"str,omitempty"`
	None bool                  `json:"none,omitempty"`
}

// Valid validates the StringFilter to ensure it has a valid configuration.
func (f StringFilter) Valid() errors.E {
	if f.Str == "" && !f.None {
		return errors.New("str or none has to be set")
	}
	if f.Str != "" && f.None {
		return errors.New("str and none cannot be both set")
	}
	return nil
}

// Filters represents a collection of search filters.
type Filters struct {
	And    []Filters     `json:"and,omitempty"`
	Or     []Filters     `json:"or,omitempty"`
	Not    *Filters      `json:"not,omitempty"`
	Rel    *RelFilter    `json:"rel,omitempty"`
	Amount *AmountFilter `json:"amount,omitempty"`
	Time   *TimeFilter   `json:"time,omitempty"`
	Str    *StringFilter `json:"str,omitempty"`
}

// Valid validates the Filters to ensure it has a valid configuration.
func (f Filters) Valid() errors.E {
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
	if nonEmpty > 1 {
		return errors.New("only one clause can be set")
	} else if nonEmpty == 0 {
		return errors.New("no clause is set")
	}
	return nil
}

// ToQuery converts the Filters to an ElasticSearch query.
func (f Filters) ToQuery() elastic.Query { //nolint:ireturn
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
	panic(errors.New("invalid filters"))
}

// Session represents a search session.
//
// A search session includes WHAT is being searched for and HOW are
// results shown/visualized, but not WHERE the user is looking at.
type Session struct {
	ID      *identifier.Identifier `json:"id"`
	Version int                    `json:"version"`
	View    ViewType               `json:"view"`
	Query   string                 `json:"query"`
	Filters *Filters               `json:"filters,omitempty"`
}

// Validate validates the Session struct.
func (s *Session) Validate(_ context.Context, existing *Session) errors.E {
	if existing == nil {
		if s.ID != nil {
			errE := errors.New("ID provided for new document")
			errors.Details(errE)["id"] = *s.ID
			return errE
		}
		id := identifier.New()
		s.ID = &id
	} else if s.ID == nil {
		// This should not really happen because we fetch existing based on i.ID.
		return errors.New("ID missing for existing document")
	} else if existing.ID == nil {
		// This should not really happen because we always store documents with ID.
		return errors.New("ID missing for existing document")
	} else if *s.ID != *existing.ID {
		// This should not really happen because we fetch existing based on i.ID.
		errE := errors.New("payload ID does not match existing ID")
		errors.Details(errE)["payload"] = *s.ID
		errors.Details(errE)["existing"] = *existing.ID
		return errE
	}

	if existing == nil {
		// We set the version to zero for new sessions.
		s.Version = 0
	} else {
		// We increase the version by one.
		// TODO: This is not race safe, needs improvement once we have storage that supports transactions.
		s.Version = existing.Version + 1
	}

	if s.Filters != nil {
		errE := s.Filters.Valid()
		if errE != nil {
			return errE
		}
	}

	if s.View == "" {
		s.View = ViewFeed
	}
	if s.View != ViewFeed && s.View != ViewTable {
		errE := errors.New("invalid view")
		errors.Details(errE)["view"] = s.View
		return errE
	}

	return nil
}

// SessionRef represents a reference to a search session.
type SessionRef struct {
	ID      identifier.Identifier `json:"id"`
	Version int                   `json:"version"`
}

// Ref returns a SessionRef reference to this Session.
func (s *Session) Ref() SessionRef {
	return SessionRef{ID: *s.ID, Version: s.Version}
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

// ToQuery converts the Session to an ElasticSearch query.
//
// TODO: Determine which operator should be the default?
// TODO: Make sure right analyzers are used for all fields.
// TODO: Limit allowed syntax for simple queries (disable fuzzy matching).
func (s *Session) ToQuery() elastic.Query { //nolint:ireturn
	boolQuery := elastic.NewBoolQuery()

	if s.Query != "" {
		boolQuery.Must(documentTextSearchQuery(s.Query, "OR"))
	}

	if s.Filters != nil {
		boolQuery.Must(s.Filters.ToQuery())
	}

	return boolQuery
}

// TODO: Use a database instead.
var searches = sync.Map{} //nolint:gochecknoglobals

// field describes a nested field for ElasticSearch to search on.
type field struct {
	Prefix string
	Field  string
}

// TODO: Return (and log) and error on invalid search requests (e.g., filters).

// CreateSession creates a new search session.
func CreateSession(ctx context.Context, session *Session) errors.E {
	errE := session.Validate(ctx, nil)
	if errE != nil {
		return errors.WrapWith(errE, ErrValidationFailed)
	}

	searches.Store(*session.ID, session)

	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit store twice?

	return nil
}

// UpdateSession updates an existing search session.
func UpdateSession(ctx context.Context, session *Session) errors.E {
	if session.ID == nil {
		return errors.WithMessage(ErrValidationFailed, "ID is missing")
	}

	// TODO: This is not race safe, needs improvement once we have storage that supports transactions.
	existingSession, errE := GetSession(ctx, *session.ID)
	if errE != nil {
		return errE
	}

	errE = session.Validate(ctx, existingSession)
	if errE != nil {
		return errors.WrapWith(errE, ErrValidationFailed)
	}

	searches.Store(*session.ID, session)

	return nil
}

// GetSessionFromID resolves an existing search session if possible.
func GetSessionFromID(ctx context.Context, value string) (*Session, errors.E) {
	id, errE := identifier.MaybeString(value)
	if errE != nil {
		return nil, errors.WrapWith(errE, ErrNotFound)
	}

	return GetSession(ctx, id)
}

// GetSession resolves an existing search session if possible.
func GetSession(_ context.Context, id identifier.Identifier) (*Session, errors.E) {
	session, ok := searches.Load(id)
	if !ok {
		return nil, errors.WithDetails(ErrNotFound, "id", id)
	}
	return session.(*Session), nil //nolint:forcetypeassert,errcheck
}

// Result represents a search result document.
type Result struct {
	ID string `json:"id"`
}

// ResultsGet retrieves search results for a given search session.
func ResultsGet(ctx context.Context, getSearchService func() (*elastic.SearchService, int64), searchSession *Session) ([]Result, map[string]interface{}, errors.E) {
	metrics := waf.MustGetMetrics(ctx)

	query := searchSession.ToQuery()

	searchService, _ := getSearchService()

	searchService = searchService.From(0).Size(MaxResultsCount).Query(query)

	m := metrics.Duration(internal.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internal.MetricElasticSearchInternal).Duration = time.Duration(res.TookInMillis) * time.Millisecond

	results := make([]Result, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		results[i] = Result{ID: hit.Id}
	}

	// Total is a string or a number.
	var total interface{}
	if res.Hits.TotalHits.Relation == "gte" {
		total = fmt.Sprintf("%d+", res.Hits.TotalHits.Value)
	} else {
		total = res.Hits.TotalHits.Value
	}

	return results, map[string]interface{}{
		"total": total,
	}, nil
}
