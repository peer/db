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

// TODO: Store the revision, license, and source used for the HTML into a meta claim.
// TODO: Investigate how to make use of additional entities metadata.
// TODO: Store categories and used templates into claims.
// TODO: Make internal links to other articles work in HTML (link to PeerDB documents instead).
// TODO: Remove links to other articles which do not exist, if there are any.
// TODO: Split article into summary and main part.
// TODO: Clean custom tags and attributes used in HTML to add metadata into HTML, potentially extract and store that.
//       See: https://www.mediawiki.org/wiki/Specs/HTML/2.4.0
// TODO: Make // links/src into https:// links/src.
// TODO: Remove some templates (e.g., infobox, top-level notices) and convert them to claims.
// TODO: Remove rendered links to categories (they should be claims).
// TODO: Extract all links pointing out of the article into claims and reverse claims (so if they point to other documents, they should have backlink as claim).
// TODO: Keep only contents of <body>.
// TODO: Skip disambiguation pages (remove corresponding document if we already have it).

func processArticle(ctx context.Context, config *Config, esClient *elastic.Client, processor *elastic.BulkProcessor, article mediawiki.Article) errors.E {
	if article.MainEntity.Identifier == "" {
		return nil
	}
	id := wikipedia.GetDocumentID(article.MainEntity.Identifier)
	esDoc, err := esClient.Get().Index("docs").Id(string(id)).Do(ctx)
	if elastic.IsNotFound(err) {
		fmt.Fprintf(os.Stderr, "document %s for entity %s for article \"%s\" not found\n", id, article.MainEntity.Identifier, article.Name)
		return nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting document %s for entity %s for article \"%s\": %s\n", id, article.MainEntity.Identifier, article.Name, err.Error())
		return nil
	} else if !esDoc.Found {
		fmt.Fprintf(os.Stderr, "document %s for entity %s for article \"%s\" not found\n", id, article.MainEntity.Identifier, article.Name)
		return nil
	}
	var document search.Document
	err = json.Unmarshal(esDoc.Source, &document)
	if err != nil {
		return errors.Errorf(`error JSON decoding document %s for entity %s for article "%s": %w`, id, article.MainEntity.Identifier, article.Name, err)
	}
	claimID := search.GetID(wikipedia.NameSpaceWikidata, id, "ARTICLE", 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim != nil {
		claim, ok := existingClaim.(*search.TextClaim)
		if !ok {
			return errors.Errorf(`document %s for entity %s for article "%s" has existing non-text claim with ID %s`, id, article.MainEntity.Identifier, article.Name, claimID)
		}
		claim.HTML["en"] = article.ArticleBody.HTML
	} else {
		claim := &search.TextClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: 1.0,
			},
			Prop: search.GetStandardPropertyReference("ARTICLE"),
			HTML: search.TranslatableHTMLString{
				"en": article.ArticleBody.HTML,
			},
		}
		err = document.Add(claim)
		if err != nil {
			fmt.Fprintf(os.Stderr, "article claim cannot be added to document %s for entity %s for article \"%s\": %s\n", id, article.MainEntity.Identifier, article.Name, err.Error())
			return nil
		}
	}
	req := elastic.NewBulkIndexRequest().Index("docs").Id(string(id)).IfSeqNo(*esDoc.SeqNo).IfPrimaryTerm(*esDoc.PrimaryTerm).Doc(&document)
	processor.Add(req)
	return nil
}
