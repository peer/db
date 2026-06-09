package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/indexer"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

type loggerAdapter struct {
	log zerolog.Logger
}

type indexConfigurationStruct struct {
	Settings map[string]any `json:"settings"`
	Mappings map[string]any `json:"mappings"`
}

// LogRoundTrip logs the request and response details using zerolog.
//
// It prefers the per-request context logger from req.Context() so that debug
// entries (including request and response bodies for successful calls) are
// buffered by the TriggerLevelWriter and only flushed when something later in
// the request emits at error level. Failed ES requests are logged at error
// level here, which both records the body details and triggers the flush of
// any prior buffered entries.
func (a loggerAdapter) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}

	log := a.log
	if req != nil {
		log = *zerolog.Ctx(req.Context())
	}

	var event *zerolog.Event
	if err != nil {
		event = log.Error().Err(err)
	} else if res != nil && res.StatusCode >= http.StatusBadRequest {
		event = log.Error()
	} else {
		event = log.Debug()
	}

	event.
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Dur("duration", dur).
		Time("start", start)

	if res != nil {
		event.Int("statusCode", res.StatusCode)
	}

	if a.RequestBodyEnabled() && req != nil && req.Body != nil && req.Body != http.NoBody {
		var buf bytes.Buffer
		if req.GetBody != nil {
			b, _ := req.GetBody()
			buf.ReadFrom(b) //nolint:errcheck,gosec
		} else {
			buf.ReadFrom(req.Body) //nolint:errcheck,gosec
		}
		addBody(event, "request", buf.Bytes())
	}

	if a.ResponseBodyEnabled() && res != nil && res.Body != nil && res.Body != http.NoBody {
		defer res.Body.Close() //nolint:errcheck
		var buf bytes.Buffer
		buf.ReadFrom(res.Body) //nolint:errcheck,gosec
		addBody(event, "response", buf.Bytes())
	}

	event.Msg("elasticsearch")

	return nil
}

// RequestBodyEnabled returns true so the transport tees the request body to us;
// the body is then attached at debug level and buffered by the context logger,
// surfacing only when a later error triggers the buffer flush.
func (a loggerAdapter) RequestBodyEnabled() bool {
	return true
}

// ResponseBodyEnabled returns true so the transport tees the response body to us;
// the body is then attached at debug level and buffered by the context logger,
// surfacing only when a later error triggers the buffer flush. ES error
// responses (4xx/5xx) are logged at error level directly, which both attaches
// the body and fires the trigger.
func (a loggerAdapter) ResponseBodyEnabled() bool {
	return true
}

// addBody attaches an HTTP body to the event. A single valid JSON document is attached as RawJSON.
// ElasticSearch bulk requests use NDJSON (one JSON document per line); we attach it as an array
// of the raw JSON documents instead. Anything else (non-JSON error pages) is attached as a plain
// string field.
func addBody(event *zerolog.Event, key string, body []byte) {
	if json.Valid(body) {
		event.RawJSON(key, body)
		return
	}
	if lines := ndjsonLines(body); len(lines) > 0 {
		if len(lines) == 1 {
			// This should probably be handled by the case above, but just in case.
			event.RawJSON(key, lines[0])
			return
		}
		arr := zerolog.Arr()
		for _, line := range lines {
			arr.RawJSON(line)
		}
		event.Array(key, arr)
		return
	}
	event.Str(key, string(body))
}

// ndjsonLines returns the non-empty lines of body if every one is a valid JSON document
// (an NDJSON body such as an ElasticSearch bulk request), or nil otherwise.
func ndjsonLines(body []byte) [][]byte {
	rawLines := bytes.Split(body, []byte("\n"))
	lines := make([][]byte, 0, len(rawLines))
	for _, line := range rawLines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !json.Valid(line) {
			return nil
		}
		lines = append(lines, line)
	}
	return lines
}

var _ elastictransport.Logger = (*loggerAdapter)(nil)

// GetClient creates and configures an Elasticsearch typed client with the specified HTTP client, logger, and URL.
func GetClient(httpClient *http.Client, logger zerolog.Logger, url string) (*elasticsearch.TypedClient, errors.E) {
	cfg := elasticsearch.Config{ //nolint:exhaustruct
		Addresses:     []string{strings.TrimSpace(url)},
		Transport:     httpClient.Transport,
		Logger:        &loggerAdapter{logger},
		AutoDrainBody: true,
		// We do not enable discovery so that Docker setup is easier.
		// TODO: Should enabling discovery be a CLI parameter?
	}
	esClient, err := elasticsearch.NewTypedClient(cfg)
	return esClient, errors.WithStack(err)
}

// LevelIndex returns the ElasticSearch index name for the given visibility level, derived from the index prefix.
func LevelIndex(indexPrefix, level string) string {
	return indexPrefix + "_" + level
}

// EnsureIndex makes sure the index for PeerDB documents exists. If not, it creates it.
// It does not update configuration of an existing index if it is different from
// what current implementation of EnsureIndex would otherwise create.
// The shards parameter specifies the number of primary shards for the index. languagePriority
// selects which languages the index mapping covers (nil yields the default all-language mapping).
func EnsureIndex(ctx context.Context, esClient *elasticsearch.TypedClient, index string, shards int, languagePriority map[string][]string) errors.E {
	exists, err := esClient.Indices.Exists(index).IsSuccess(ctx)
	if err != nil {
		errE := WithESError(err)
		errors.Details(errE)["index"] = index
		return errE
	}

	if !exists {
		indexConfiguration, errE := Mapping(languagePriority)
		if errE != nil {
			return errE
		}
		var config indexConfigurationStruct
		errE = x.UnmarshalWithoutUnknownFields(indexConfiguration, &config)
		if errE != nil {
			return errE
		}

		config.Settings["number_of_shards"] = shards
		config.Settings["number_of_replicas"] = 0

		configJSON, errE := x.MarshalWithoutEscapeHTML(config)
		if errE != nil {
			return errE
		}

		createIndex, err := esClient.Indices.Create(index).Raw(bytes.NewReader(configJSON)).Do(ctx)
		if err != nil {
			errE := WithESError(err)
			errors.Details(errE)["index"] = index
			return errE
		}
		if !createIndex.Acknowledged {
			// TODO: Wait for acknowledgment using Task API?
			errE := errors.New("create index not acknowledged")
			errors.Details(errE)["index"] = index
			return errE
		}
	}

	return nil
}

// NewHTTPClient creates a retryable HTTP client with the specified base HTTP client and logger.
func NewHTTPClient(logger zerolog.Logger, httpClient *http.Client) *http.Client {
	// TODO: Make contact e-mail into a CLI argument.
	return indexer.NewHTTPClient(logger, httpClient, fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", cli.Version, cli.BuildTimestamp, cli.Revision)) //nolint:lll
}

const fetchDocumentIDsPageSize = 5000

// rawFieldValue wraps a types.FieldValue so it satisfies types.FieldValueVariant.
//
// See: https://github.com/elastic/go-elasticsearch/issues/1328
type rawFieldValue struct {
	V types.FieldValue
}

// FieldValueCaster returns the wrapped FieldValue pointer.
func (r *rawFieldValue) FieldValueCaster() *types.FieldValue {
	return &r.V
}

// FetchDocumentIDs retrieves document IDs matching the given class IDs using ES PIT.
// If classIDs is empty, all documents are returned.
func FetchDocumentIDs(ctx context.Context, esClient *elasticsearch.TypedClient, index string, classIDs []identifier.Identifier) ([]identifier.Identifier, errors.E) {
	pit, err := esClient.OpenPointInTime(index).KeepAlive("1m").Do(ctx)
	if err != nil {
		return nil, WithESError(err)
	}
	pitID := pit.Id

	defer func() {
		_, _ = esClient.ClosePointInTime().Id(pitID).Do(ctx)
	}()

	// Build query.
	var query types.QueryVariant
	if len(classIDs) == 0 {
		query = esdsl.NewMatchAllQuery()
	} else if len(classIDs) == 1 {
		boolQ := esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(internalCore.InstanceOfPropID.String())),
			esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(classIDs[0].String())),
		)
		query = esdsl.NewNestedQuery(boolQ).Path("claims.ref")
	} else {
		shoulds := make([]types.QueryVariant, 0, len(classIDs))
		for _, classID := range classIDs {
			boolQ := esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(internalCore.InstanceOfPropID.String())),
				esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(classID.String())),
			)
			shoulds = append(shoulds, esdsl.NewNestedQuery(boolQ).Path("claims.ref"))
		}
		query = esdsl.NewBoolQuery().Should(shoulds...)
	}

	var allIDs []identifier.Identifier
	var searchAfter []types.FieldValue

	for {
		searchService := esClient.Search().
			Source_(esdsl.NewSourceConfig().Bool(false)).
			AllowPartialSearchResults(false).
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
			return nil, WithESError(err)
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
