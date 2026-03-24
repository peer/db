package peerdb

import (
	"context"
	"io"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"
	"gopkg.in/yaml.v3"

	"gitlab.com/peerdb/peerdb/base"
	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
)

// Build contains version and build metadata.
type Build struct {
	Version        string `json:"version,omitempty"`
	BuildTimestamp string `json:"buildTimestamp,omitempty"`
	Revision       string `json:"revision,omitempty"`
}

// Site represents a single site in the PeerDB application with its configuration and state.
type Site struct {
	waf.Site `yaml:",inline"`

	Build *Build `json:"build,omitempty" yaml:"-"`

	Index  string `json:"index,omitempty"  yaml:"index,omitempty"`
	Schema string `json:"schema,omitempty" yaml:"schema,omitempty"`
	Title  string `json:"title,omitempty"  yaml:"title,omitempty"`

	LanguagePriority map[string][]string `json:"languagePriority,omitempty" yaml:"languagePriority,omitempty"`

	Base        *base.B                    `json:"-" yaml:"-"`
	DBPool      *pgxpool.Pool              `json:"-" yaml:"-"`
	ESClient    *elasticsearch.TypedClient `json:"-" yaml:"-"`
	RiverClient *river.Client[pgx.Tx]      `json:"-" yaml:"-"`

	initialized bool

	// TODO: How to keep propertiesTotal and unitsTotal in sync with the number of properties and units available, if they are added or removed after initialization?
	propertiesTotal int64
	unitsTotal      int64
}

// Decode implements kong.MapperValue to decode Site from JSON/YAML configuration.
func (s *Site) Decode(ctx *kong.DecodeContext) error {
	var value string
	err := ctx.Scan.PopValueInto("value", &value)
	if err != nil {
		return errors.WithStack(err)
	}
	decoder := yaml.NewDecoder(strings.NewReader(value))
	decoder.KnownFields(true)
	err = decoder.Decode(s)
	if err != nil {
		if yamlErr, ok := errors.AsType[*yaml.TypeError](err); ok {
			e := "error"
			if len(yamlErr.Errors) > 1 {
				e = "errors"
			}
			return errors.Errorf("yaml: unmarshal %s: %s", e, strings.Join(yamlErr.Errors, "; "))
		} else if errors.Is(err, io.EOF) {
			return nil
		}
		return errors.WithStack(err)
	}
	return nil
}

const fetchDocumentIDsPageSize = 5000

//nolint:gochecknoglobals
var (
	instanceOfPropID = identifier.From(core.Namespace, "INSTANCE_OF").String()
)

// rawFieldValue wraps a types.FieldValue so it satisfies types.FieldValueVariant.
//
// See: https://github.com/elastic/go-elasticsearch/issues/1328
type rawFieldValue struct {
	v types.FieldValue
}

func (r *rawFieldValue) FieldValueCaster() *types.FieldValue {
	return &r.v
}

func (s *Site) fetchDocumentIDs(ctx context.Context, classID identifier.Identifier) ([]identifier.Identifier, errors.E) {
	boolQuery := esdsl.NewBoolQuery().Must(
		esdsl.NewTermQuery("claims.rel.prop", esdsl.NewFieldValue().String(instanceOfPropID)),
		esdsl.NewTermQuery("claims.rel.to", esdsl.NewFieldValue().String(classID.String())),
	)
	query := esdsl.NewNestedQuery(boolQuery).Path("claims.rel")

	pit, err := s.ESClient.OpenPointInTime(s.Index).KeepAlive("1m").Do(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	pitID := pit.Id

	defer func() {
		_, _ = s.ESClient.ClosePointInTime().Id(pitID).Do(ctx)
	}()

	var allIDs []identifier.Identifier
	var searchAfter []types.FieldValue

	for {
		searchService := s.ESClient.Search().Source_(esdsl.NewSourceConfig().Bool(false)).AllowPartialSearchResults(false).
			Query(query).
			Size(fetchDocumentIDsPageSize).
			Pit(esdsl.NewPointInTimeReference().Id(pitID).KeepAlive(esdsl.NewDuration().String("1m"))).
			Sort(esdsl.NewSortOptions().AddSortOption("_shard_doc", esdsl.NewFieldSort(sortorder.Asc)))

		if searchAfter != nil {
			args := make([]types.FieldValueVariant, 0, len(searchAfter))
			for _, v := range searchAfter {
				args = append(args, &rawFieldValue{v})
			}
			searchService = searchService.SearchAfter(args...)
		}

		res, err := searchService.Do(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		hits := res.Hits.Hits

		for _, hit := range hits {
			if hit.Id_ == nil {
				return nil, errors.New("hit has no ID")
			}
			id, errE := identifier.MaybeString(*hit.Id_)
			if errE != nil {
				return nil, errE
			}
			allIDs = append(allIDs, id)
		}

		if len(hits) < fetchDocumentIDsPageSize {
			break
		}

		lastHit := hits[len(hits)-1]
		searchAfter = lastHit.Sort
	}

	return allIDs, nil
}

func (s *Site) fetchDocuments(ctx context.Context, classID identifier.Identifier) ([]*document.D, errors.E) {
	allIDs, errE := s.fetchDocumentIDs(ctx, classID)
	if errE != nil {
		return nil, errE
	}

	documents := make([]*document.D, 0, len(allIDs))
	for _, id := range allIDs {
		doc, _, _, _, errE := s.Base.GetDocumentLatestDoc(ctx, id)
		if errE != nil {
			return nil, errE
		}
		documents = append(documents, doc)
	}

	return documents, nil
}

func (s *Site) updatePropertiesTotal(_ context.Context, documents []*document.D) errors.E {
	// TODO: Limit properties only to those really used in filters ("rel", "amount", "amountRange")?
	// TODO: Limit really only to properties.
	s.propertiesTotal = int64(len(documents))
	return nil
}

func (s *Site) updateUnitsTotal(_ context.Context, documents []*document.D) errors.E {
	// TODO: Limit really only to units.
	s.unitsTotal = int64(len(documents))
	return nil
}

// Start starts the base for the site.
//
// You have to call this or PopulateAndStart for each site after Init.
func (s *Site) Start(ctx context.Context, documents []*document.D) errors.E {
	errE := s.updatePropertiesTotal(ctx, documents)
	if errE != nil {
		return errE
	}

	errE = s.updateUnitsTotal(ctx, documents)
	if errE != nil {
		return errE
	}

	return s.Base.Start(ctx, documents)
}

// PopulateAndStart for the site: inserts the given documents into the store, starts the base,
// waits for Elasticsearch to catch up, and then refreshes ElasticSearch index.
//
// You have to call this or Start for each site after Init.
func (s *Site) PopulateAndStart(ctx context.Context, documents []*document.D, progress func(doc *document.D)) errors.E {
	errE := s.updatePropertiesTotal(ctx, documents)
	if errE != nil {
		return errE
	}

	errE = s.updateUnitsTotal(ctx, documents)
	if errE != nil {
		return errE
	}

	return s.Base.PopulateAndStart(ctx, documents, progress)
}
