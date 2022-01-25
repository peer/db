package main

import (
	"context"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

func processEntity(ctx context.Context, config *Config, client *retryablehttp.Client, processor *elastic.BulkProcessor, entity mediawiki.Entity) errors.E {
	document, err := wikipedia.ConvertEntity(ctx, client, entity)
	if errors.Is(err, wikipedia.NotSupportedError) {
		return nil
	} else if err != nil {
		return err
	}

	saveDocument(config, processor, document)

	return nil
}

func saveDocument(config *Config, processor *elastic.BulkProcessor, doc *search.Document) {
	req := elastic.NewBulkIndexRequest().Index("docs").Id(string(doc.ID)).Doc(doc)
	processor.Add(req)
}
