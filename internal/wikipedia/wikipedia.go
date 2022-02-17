package wikipedia

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
)

var NameSpaceWikipediaFile = uuid.MustParse("94b1c372-bc28-454c-a45a-2e4d29d15146")

func ConvertWikipediaImage(ctx context.Context, httpClient *retryablehttp.Client, image Image) (*search.Document, errors.E) {
	return convertImage(ctx, httpClient, NameSpaceWikipediaFile, "en", "en.wikipedia.org", "ENGLISH_WIKIPEDIA", image)
}

// TODO: Store the revision, license, and source used for the HTML into a meta claim.
// TODO: Investigate how to make use of additional entities metadata.
//       See: https://www.mediawiki.org/wiki/Topic:Wotwu75akwx2wnsb
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
func ConvertWikipediaArticle(document *search.Document, namespace uuid.UUID, id string, article mediawiki.Article) errors.E {
	claimID := search.GetID(namespace, id, "ARTICLE", 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim != nil {
		claim, ok := existingClaim.(*search.TextClaim)
		if !ok {
			fmt.Fprintf(
				os.Stderr,
				"unexpected: document %s for source ID \"%s\" for article \"%s\" has existing non-text claim with ID %s",
				document.ID, id, article.Name, claimID,
			)
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
		err := document.Add(claim)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"unexpected: article claim cannot be added to document %s for source ID \"%s\" for article \"%s\": %s\n",
				document.ID, id, article.Name, err.Error(),
			)
		}
	}

	return nil
}
