package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/wikipedia"
)

func processArticle(ctx context.Context, config *Config, esClient *elastic.Client, processor *elastic.BulkProcessor, article mediawiki.Article) errors.E {
	id := wikipedia.GetDocumentID(article.MainEntity.Identifier)
	esDoc, err := esClient.Get().Index("docs").Id(string(id)).Do(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting document %s for entity %s for article \"%s\": %s\n", id, article.MainEntity.Identifier, article.Name, err.Error())
		return nil
	}
	var document search.Document
	err = json.Unmarshal(esDoc.Source, &document)
	if err != nil {
		return errors.Errorf(`error JSON decoding document %s for entity %s for article "%s": %w`, id, article.MainEntity.Identifier, article.Name, err)
	}
	claim := search.TextClaim{
		CoreClaim: search.CoreClaim{
			ID:         search.GetID(wikipedia.NameSpaceWikidata, id, "ARTICLE", 0),
			Confidence: 1.0,
		},
		Prop: search.GetStandardPropertyReference("ARTICLE"),
		HTML: search.TranslatableHTMLString{
			"en": article.ArticleBody.HTML,
		},
	}
	// TODO: Make sure we are not adding it multiple times, but we change any existing one.
	err = document.Add(claim)
	if err != nil {
		fmt.Fprintf(os.Stderr, "article claim cannot be added to document %s for entity %s for article \"%s\": %s\n", id, article.MainEntity.Identifier, article.Name, err.Error())
		return nil
	}
	req := elastic.NewBulkIndexRequest().Index("docs").Id(string(id)).IfSeqNo(*esDoc.SeqNo).IfPrimaryTerm(*esDoc.PrimaryTerm).Doc(&document)
	processor.Add(req)
	return nil
}
