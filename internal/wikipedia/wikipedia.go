package wikipedia

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"

	"gitlab.com/peerdb/search"
)

var (
	NameSpaceWikipediaFile = uuid.MustParse("94b1c372-bc28-454c-a45a-2e4d29d15146")

	WikimediaCommonsFileError = errors.Base("file is from Wikimedia Commons error")
)

func ConvertWikipediaImage(ctx context.Context, httpClient *retryablehttp.Client, token string, apiLimit int, image Image) (*search.Document, errors.E) {
	return convertImage(ctx, httpClient, NameSpaceWikipediaFile, "en", "en.wikipedia.org", "ENGLISH_WIKIPEDIA", token, apiLimit, image)
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
			errE := errors.New("unexpected article claim type")
			errors.Details(errE)["doc"] = string(document.ID)
			errors.Details(errE)["claim"] = string(claimID)
			errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.TextClaim{})
			errors.Details(errE)["title"] = article.Name
			return errE
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
			errE := errors.WithMessage(err, "claim cannot be added")
			errors.Details(errE)["doc"] = string(document.ID)
			errors.Details(errE)["claim"] = string(claimID)
			errors.Details(errE)["title"] = article.Name
			return errE
		}
	}

	return nil
}

// TODO: Should we use cache for cases where file has not been found?
//       Currently we use the function in the context where every file document is fetched
//       only once, one after the other, so caching will not help.

// We do not follow a redirect, because currently we use the function in
// the context where we want the document exactly under that name
// (to add its article). Otherwise it could happen that we add an article
// with only a redirect tag to a document which has a proper article,
// overwriting it (redirect pages also have articles).
func GetWikipediaFile(
	ctx context.Context, log zerolog.Logger, httpClient *retryablehttp.Client, esClient *elastic.Client, token string, apiLimit int, name string,
) (*search.Document, *elastic.SearchHit, errors.E) {
	document, hit, errE := getDocumentFromES(ctx, esClient, "ENGLISH_WIKIPEDIA_FILE_NAME", name)
	if errors.Is(errE, NotFoundError) {
		// Passthrough.
	} else if errE != nil {
		errors.Details(errE)["file"] = name
		return nil, nil, errE
	} else {
		return document, hit, nil
	}

	// Is there a Wikimedia Commons file under that name? Most files with article on Wikipedia
	// are in fact from Wikimedia Commons and have the same name. We check it here this way so that we
	// do not have to hit Wikipedia API too often. There can also be files on Wikipedia from Wikimedia
	// Commons which have different names so this can have false negatives. False positives might also
	// be possible but are probably harmless: we already did not find a Wikipedia file, so we are
	// primarily trying to understand why not.
	_, _, errE = getDocumentFromES(ctx, esClient, "WIKIMEDIA_COMMONS_FILE_NAME", name)
	if errors.Is(errE, NotFoundError) {
		// Passthrough.
	} else if errE != nil {
		errors.Details(errE)["file"] = name
		return nil, nil, errors.WithMessage(errE, "checking for Wikimedia Commons")
	} else {
		errE := errors.WithStack(WikimediaCommonsFileError) //nolint:govet
		errors.Details(errE)["file"] = name
		errors.Details(errE)["url"] = fmt.Sprintf("https://commons.wikimedia.org/wiki/File:%s", name)
		return nil, nil, errE
	}

	// We could not find the file. Maybe there it is from Wikimedia Commons?
	ii, errE := getImageInfoForFilename(ctx, httpClient, "en.wikipedia.org", token, apiLimit, name)
	if errE != nil {
		// Not found error here probably means that file has been deleted recently.
		errE := errors.WithMessage(errE, "checking API") //nolint:govet
		errors.Details(errE)["file"] = name
		return nil, nil, errE
	}

	descriptionURL, err := url.Parse(ii.DescriptionURL)
	if err != nil {
		errE := errors.WithMessage(err, "checking API") //nolint:govet
		errors.Details(errE)["file"] = name
		errors.Details(errE)["url"] = ii.DescriptionURL
		return nil, nil, errE
	}

	if descriptionURL.Host != "en.wikipedia.org" {
		descriptionFilename := strings.TrimPrefix(descriptionURL.Path, "/wiki/File:")
		if descriptionFilename != name {
			log.Warn().Str("file", name).Str("commons", descriptionFilename).Msg("Wikipedia file name mismatch with Mediawiki Commons")
		}

		errE := errors.WithStack(WikimediaCommonsFileError) //nolint:govet
		errors.Details(errE)["file"] = name
		errors.Details(errE)["url"] = ii.DescriptionURL
		return nil, nil, errE
	}

	// File exists through API but we do not have it. Probably it is too new.
	errE = errors.WithStack(NotFoundError)
	errors.Details(errE)["file"] = name
	return nil, nil, errE
}
