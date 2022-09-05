package search

import (
	"context"
	"io"
	"net/http"
	"strconv"

	gddo "github.com/golang/gddo/httputil"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

func (s *Service) populateProperties(ctx context.Context) errors.E {
	boolQuery := elastic.NewBoolQuery().Must(
		elastic.NewTermQuery("active.rel.prop._id", "2fjzZyP7rv8E4aHnBc6KAa"),
		elastic.NewTermQuery("active.rel.to._id", "HohteEmv2o7gPRnJ5wukVe"),
	)
	query := elastic.NewNestedQuery("active.rel", boolQuery)

	scroll := s.ESClient.Scroll(s.Index).Size(1000).Sort("_doc", true).
		SearchSource(elastic.NewSearchSource().Query(query))

	results := []DocumentReference{}
	for {
		res, err := scroll.Do(ctx)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return errors.WithStack(err)
		}

		for _, hit := range res.Hits.Hits {
			var document Document
			errE := x.UnmarshalWithoutUnknownFields(hit.Source, &document)
			if errE != nil {
				errors.Details(errE)["doc"] = hit.Id
				return errE
			}

			// ID is not stored in the document, so we set it here ourselves.
			document.ID = Identifier(hit.Id)

			results = append(results, document.Reference())
		}
	}

	encoded, err := x.MarshalWithoutEscapeHTML(results)
	if err != nil {
		return errors.WithStack(err)
	}

	total := strconv.Itoa(len(results))

	s.properties = encoded
	s.propertiesTotal = total

	return nil
}

func (s *Service) DocumentSearchPropertiesGetJSON(w http.ResponseWriter, req *http.Request, _ Params) {
	contentEncoding := gddo.NegotiateContentEncoding(req, allCompressions)
	if contentEncoding == "" {
		s.NotAcceptable(w, req, nil)
		return
	}

	metadata := http.Header{
		"Total": {s.propertiesTotal},
	}

	s.writeJSON(w, req, contentEncoding, s.properties, metadata)
}
