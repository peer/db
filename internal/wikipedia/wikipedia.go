package wikipedia

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
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

func ConvertWikipediaImage(
	ctx context.Context, log zerolog.Logger, httpClient *retryablehttp.Client, token string, apiLimit int, image Image, document *search.Document,
) errors.E {
	return convertImage(ctx, log, httpClient, NameSpaceWikipediaFile, "en", "en.wikipedia.org", token, apiLimit, image, document)
}

// TODO: Store the revision, license, and source used for the HTML into a meta claim.
// TODO: Investigate how to make use of additional entities metadata.
//       See: https://www.mediawiki.org/wiki/Topic:Wotwu75akwx2wnsb
// TODO: Make internal links to other articles work in HTML (link to PeerDB documents instead).
// TODO: Remove links to other articles which do not exist, if there are any.
// TODO: Clean custom tags and attributes used in HTML to add metadata into HTML, potentially extract and store that.
//       See: https://www.mediawiki.org/wiki/Specs/HTML/2.4.0
// TODO: Remove some templates (e.g., infobox, top-level notices) and convert them to claims.
// TODO: Extract all links pointing out of the article into claims and reverse claims (so if they point to other documents, they should have backlink as claim).
func ConvertWikipediaArticle(namespace uuid.UUID, id string, article mediawiki.Article, document *search.Document) errors.E {
	body, doc, err := ExtractArticle(article.ArticleBody.HTML)
	if err != nil {
		errE := errors.WithMessage(err, "article extraction failed")
		errors.Details(errE)["doc"] = string(document.ID)
		errors.Details(errE)["title"] = article.Name
		return errE
	}

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
		claim.HTML["en"] = body
	} else {
		claim := &search.TextClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: HighConfidence,
			},
			Prop: search.GetStandardPropertyReference("ARTICLE"),
			HTML: search.TranslatableHTMLString{
				"en": body,
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

	claimID = search.GetID(namespace, id, "HAS_ARTICLE", 0)
	existingClaim = document.GetByID(claimID)
	if existingClaim == nil {
		claim := &search.LabelClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: HighConfidence,
			},
			Prop: search.GetStandardPropertyReference("HAS_ARTICLE"),
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

	claimID = search.GetID(namespace, id, "ENGLISH_WIKIPEDIA_PAGE_ID", 0)
	existingClaim = document.GetByID(claimID)
	if existingClaim != nil {
		claim, ok := existingClaim.(*search.IdentifierClaim)
		if !ok {
			errE := errors.New("unexpected English Wikipedia page id claim type")
			errors.Details(errE)["doc"] = string(document.ID)
			errors.Details(errE)["claim"] = string(claimID)
			errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.IdentifierClaim{})
			errors.Details(errE)["title"] = article.Name
			return errE
		}
		claim.Identifier = strconv.FormatInt(article.Identifier, 10)
	} else {
		claim := &search.IdentifierClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: HighConfidence,
			},
			Prop:       search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_PAGE_ID"),
			Identifier: strconv.FormatInt(article.Identifier, 10),
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

	summary, err := ExtractArticleSummary(doc)
	if err != nil {
		errE := errors.WithMessage(err, "summary extraction failed")
		errors.Details(errE)["doc"] = string(document.ID)
		errors.Details(errE)["title"] = article.Name
		return errE
	}

	// TODO: Remove summary if is now empty, but before it was not.
	if summary != "" {
		// A slightly different construction for claimID so that it does not overlap with any other descriptions.
		claimID = search.GetID(namespace, id, "ARTICLE", 0, "DESCRIPTION", 0)
		existingClaim = document.GetByID(claimID)
		if existingClaim != nil {
			claim, ok := existingClaim.(*search.TextClaim)
			if !ok {
				errE := errors.New("unexpected description claim type")
				errors.Details(errE)["doc"] = string(document.ID)
				errors.Details(errE)["claim"] = string(claimID)
				errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
				errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.TextClaim{})
				errors.Details(errE)["title"] = article.Name
				return errE
			}
			claim.HTML["en"] = summary
		} else {
			claim := &search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         claimID,
					Confidence: HighConfidence,
				},
				Prop: search.GetStandardPropertyReference("DESCRIPTION"),
				HTML: search.TranslatableHTMLString{
					"en": summary,
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
	}

	return nil
}

func ConvertWikipediaFileDescription(namespace uuid.UUID, id string, article mediawiki.Article, document *search.Document) errors.E {
	descriptions, err := ExtractFileDescriptions(article.ArticleBody.HTML)
	if err != nil {
		errE := errors.WithMessage(err, "descriptions extraction failed")
		errors.Details(errE)["doc"] = string(document.ID)
		errors.Details(errE)["title"] = article.Name
		return errE
	}

	claimID := search.GetID(namespace, id, "ENGLISH_WIKIPEDIA_PAGE_ID", 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim != nil {
		claim, ok := existingClaim.(*search.IdentifierClaim)
		if !ok {
			errE := errors.New("unexpected English Wikipedia page id claim type")
			errors.Details(errE)["doc"] = string(document.ID)
			errors.Details(errE)["claim"] = string(claimID)
			errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.IdentifierClaim{})
			errors.Details(errE)["title"] = article.Name
			return errE
		}
		claim.Identifier = strconv.FormatInt(article.Identifier, 10)
	} else {
		claim := &search.IdentifierClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: HighConfidence,
			},
			Prop:       search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_PAGE_ID"),
			Identifier: strconv.FormatInt(article.Identifier, 10),
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

	// TODO: Remove old descriptions if there are now less of them then before.
	for i, description := range descriptions {
		// A slightly different construction for claimID so that it does not overlap with any other descriptions.
		claimID = search.GetID(namespace, id, "FILE", 0, "DESCRIPTION", i)
		existingClaim = document.GetByID(claimID)
		if existingClaim != nil {
			claim, ok := existingClaim.(*search.TextClaim)
			if !ok {
				errE := errors.New("unexpected description claim type")
				errors.Details(errE)["doc"] = string(document.ID)
				errors.Details(errE)["claim"] = string(claimID)
				errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
				errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.TextClaim{})
				errors.Details(errE)["title"] = article.Name
				return errE
			}
			claim.HTML["en"] = description
		} else {
			claim := &search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         claimID,
					Confidence: HighConfidence,
				},
				Prop: search.GetStandardPropertyReference("DESCRIPTION"),
				HTML: search.TranslatableHTMLString{
					"en": description,
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
	}

	return nil
}

func ConvertWikipediaCategoryArticle(log zerolog.Logger, namespace uuid.UUID, id string, article mediawiki.Article, document *search.Document) errors.E {
	description, err := ExtractCategoryDescription(article.ArticleBody.HTML)
	if err != nil {
		errE := errors.WithMessage(err, "description extraction failed")
		errors.Details(errE)["doc"] = string(document.ID)
		errors.Details(errE)["title"] = article.Name
		return errE
	}

	claimID := search.GetID(namespace, id, "ENGLISH_WIKIPEDIA_PAGE_ID", 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim != nil {
		claim, ok := existingClaim.(*search.IdentifierClaim)
		if !ok {
			errE := errors.New("unexpected English Wikipedia page id claim type")
			errors.Details(errE)["doc"] = string(document.ID)
			errors.Details(errE)["claim"] = string(claimID)
			errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.IdentifierClaim{})
			errors.Details(errE)["title"] = article.Name
			return errE
		}
		claim.Identifier = strconv.FormatInt(article.Identifier, 10)
	} else {
		claim := &search.IdentifierClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: HighConfidence,
			},
			Prop:       search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_PAGE_ID"),
			Identifier: strconv.FormatInt(article.Identifier, 10),
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

	// TODO: Remove description if is now empty, but before it was not.
	if description != "" {
		// A slightly different construction for claimID so that it does not overlap with any other descriptions.
		claimID = search.GetID(namespace, id, "ARTICLE", 0, "DESCRIPTION", 0)
		existingClaim = document.GetByID(claimID)
		if existingClaim != nil {
			claim, ok := existingClaim.(*search.TextClaim)
			if !ok {
				errE := errors.New("unexpected description claim type")
				errors.Details(errE)["doc"] = string(document.ID)
				errors.Details(errE)["claim"] = string(claimID)
				errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
				errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.TextClaim{})
				errors.Details(errE)["title"] = article.Name
				return errE
			}
			claim.HTML["en"] = description
		} else {
			claim := &search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         claimID,
					Confidence: HighConfidence,
				},
				Prop: search.GetStandardPropertyReference("DESCRIPTION"),
				HTML: search.TranslatableHTMLString{
					"en": description,
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
	}

	return nil
}

func ConvertWikipediaTemplateArticle(namespace uuid.UUID, id string, page AllPagesPage, html string, document *search.Document) errors.E {
	description, err := ExtractTemplateDescription(html)
	if err != nil {
		errE := errors.WithMessage(err, "description extraction failed")
		errors.Details(errE)["doc"] = string(document.ID)
		errors.Details(errE)["title"] = page.Title
		return errE
	}

	claimID := search.GetID(namespace, id, "ENGLISH_WIKIPEDIA_PAGE_ID", 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim != nil {
		claim, ok := existingClaim.(*search.IdentifierClaim)
		if !ok {
			errE := errors.New("unexpected English Wikipedia page id claim type")
			errors.Details(errE)["doc"] = string(document.ID)
			errors.Details(errE)["claim"] = string(claimID)
			errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.IdentifierClaim{})
			errors.Details(errE)["title"] = page.Title
			return errE
		}
		claim.Identifier = strconv.FormatInt(page.Identifier, 10)
	} else {
		claim := &search.IdentifierClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: HighConfidence,
			},
			Prop:       search.GetStandardPropertyReference("ENGLISH_WIKIPEDIA_PAGE_ID"),
			Identifier: strconv.FormatInt(page.Identifier, 10),
		}
		err := document.Add(claim)
		if err != nil {
			errE := errors.WithMessage(err, "claim cannot be added")
			errors.Details(errE)["doc"] = string(document.ID)
			errors.Details(errE)["claim"] = string(claimID)
			errors.Details(errE)["title"] = page.Title
			return errE
		}
	}

	// TODO: Remove description if is now empty, but before it was not.
	if description != "" {
		// A slightly different construction for claimID so that it does not overlap with any other descriptions.
		claimID = search.GetID(namespace, id, "ARTICLE", 0, "DESCRIPTION", 0)
		existingClaim = document.GetByID(claimID)
		if existingClaim != nil {
			claim, ok := existingClaim.(*search.TextClaim)
			if !ok {
				errE := errors.New("unexpected description claim type")
				errors.Details(errE)["doc"] = string(document.ID)
				errors.Details(errE)["claim"] = string(claimID)
				errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
				errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.TextClaim{})
				errors.Details(errE)["title"] = page.Title
				return errE
			}
			claim.HTML["en"] = description
		} else {
			claim := &search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         claimID,
					Confidence: HighConfidence,
				},
				Prop: search.GetStandardPropertyReference("DESCRIPTION"),
				HTML: search.TranslatableHTMLString{
					"en": description,
				},
			}
			err := document.Add(claim)
			if err != nil {
				errE := errors.WithMessage(err, "claim cannot be added")
				errors.Details(errE)["doc"] = string(document.ID)
				errors.Details(errE)["claim"] = string(claimID)
				errors.Details(errE)["title"] = page.Title
				return errE
			}
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
	ctx context.Context, index string, log zerolog.Logger, httpClient *retryablehttp.Client, esClient *elastic.Client, token string, apiLimit int, name string,
) (*search.Document, *elastic.SearchHit, errors.E) {
	document, hit, errE := getDocumentFromES(ctx, index, esClient, "ENGLISH_WIKIPEDIA_FILE_NAME", name)
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
	_, _, errE = getDocumentFromES(ctx, index, esClient, "WIKIMEDIA_COMMONS_FILE_NAME", name)
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

	// We could not find the file. Maybe it is from Wikimedia Commons?
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

// TODO: How to remove categories which has previously been added but are later on removed?
func ConvertWikipediaArticleCategories(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client,
	namespace uuid.UUID, id string, article mediawiki.Article, document *search.Document,
) errors.E {
	for _, category := range article.Categories {
		convertWikipediaCategory(ctx, index, log, esClient, namespace, id, article.Name, category.Name, document)
	}
	return nil
}

// TODO: How to remove templates which has previously been added but are later on removed?
func ConvertWikipediaArticleTemplates(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client,
	namespace uuid.UUID, id string, article mediawiki.Article, document *search.Document,
) errors.E {
	for _, template := range article.Templates {
		convertWikipediaTemplate(ctx, index, log, esClient, namespace, id, article.Name, template.Name, document)
	}
	return nil
}

// TODO: How to remove categories which has previously been added but are later on removed?
func ConvertWikipediaPageCategories(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client,
	namespace uuid.UUID, id string, page AllPagesPage, document *search.Document,
) errors.E {
	for _, category := range page.Categories {
		convertWikipediaCategory(ctx, index, log, esClient, namespace, id, page.Title, category.Title, document)
	}
	return nil
}

// TODO: How to remove templates which has previously been added but are later on removed?
func ConvertWikipediaPageTemplates(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client,
	namespace uuid.UUID, id string, page AllPagesPage, document *search.Document,
) errors.E {
	for _, template := range page.Templates {
		convertWikipediaTemplate(ctx, index, log, esClient, namespace, id, page.Title, template.Title, document)
	}
	return nil
}

func convertWikipediaCategory(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client,
	namespace uuid.UUID, id, title, category string, document *search.Document,
) {
	if !strings.HasPrefix(category, "Category:") {
		return
	}

	document, _, err := getDocumentFromES(ctx, index, esClient, "ENGLISH_WIKIPEDIA_ARTICLE_TITLE", category)
	if err != nil {
		log.Error().Str("doc", string(document.ID)).Str("entity", id).Str("title", title).Str("category", category).
			Err(err).Fields(errors.AllDetails(err)).Msg("unable to find category")
		return
	}

	claimID := search.GetID(namespace, id, "category", string(document.ID), 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim == nil {
		claim := &search.LabelClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: HighConfidence,
			},
			Prop: search.DocumentReference{
				ID:     document.ID,
				Name:   document.Name,
				Score:  document.Score,
				Scores: document.Scores,
			},
		}
		err := document.Add(claim)
		if err != nil {
			log.Error().Str("doc", string(document.ID)).Str("entity", id).Str("claim", string(claimID)).Str("title", title).
				Err(err).Fields(errors.AllDetails(err)).Msg("claim cannot be added")
		}
	}
}

func convertWikipediaTemplate(
	ctx context.Context, index string, log zerolog.Logger, esClient *elastic.Client,
	namespace uuid.UUID, id, title, template string, document *search.Document,
) {
	if !strings.HasPrefix(template, "Template:") && !strings.HasPrefix(template, "Module:") {
		return
	}

	document, _, err := getDocumentFromES(ctx, index, esClient, "ENGLISH_WIKIPEDIA_ARTICLE_TITLE", template)
	if err != nil {
		log.Error().Str("doc", string(document.ID)).Str("entity", id).Str("title", title).Str("template", template).
			Err(err).Fields(errors.AllDetails(err)).Msg("unable to find template")
		return
	}

	claimID := search.GetID(namespace, id, "USES_MEDIAWIKI_TEMPLATE", string(document.ID), 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim == nil {
		claim := &search.RelationClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: HighConfidence,
			},
			Prop: search.GetStandardPropertyReference("USES_MEDIAWIKI_TEMPLATE"),
			To: search.DocumentReference{
				ID:     document.ID,
				Name:   document.Name,
				Score:  document.Score,
				Scores: document.Scores,
			},
		}
		err := document.Add(claim)
		if err != nil {
			log.Error().Str("doc", string(document.ID)).Str("entity", id).Str("claim", string(claimID)).Str("title", title).
				Err(err).Fields(errors.AllDetails(err)).Msg("claim cannot be added")
		}
	}
}

// TODO: How to remove redirects which has previously been added but are later on removed?
func ConvertArticleRedirects(log zerolog.Logger, namespace uuid.UUID, id string, article mediawiki.Article, document *search.Document) errors.E {
	for _, redirect := range article.Redirects {
		convertRedirect(log, namespace, id, article.Name, redirect.Name, document)
	}
	return nil
}

// TODO: How to remove redirects which has previously been added but are later on removed?
func ConvertPageRedirects(log zerolog.Logger, namespace uuid.UUID, id string, page AllPagesPage, document *search.Document) errors.E {
	for _, redirect := range page.Redirects {
		convertRedirect(log, namespace, id, page.Title, redirect.Title, document)
	}
	return nil
}

func convertRedirect(log zerolog.Logger, namespace uuid.UUID, id, title, redirect string, document *search.Document) {
	claimID := search.GetID(namespace, id, "ALSO_KNOWN_AS", redirect)
	existingClaim := document.GetByID(claimID)
	if existingClaim != nil {
		return
	}
	escapedName := html.EscapeString(redirect)
	found := false
	for _, claim := range document.Get(search.GetStandardPropertyID("ALSO_KNOWN_AS")) {
		if c, ok := claim.(*search.TextClaim); ok && c.HTML["en"] == escapedName {
			found = true
			break
		}
	}
	if found {
		return
	}
	claim := &search.TextClaim{
		CoreClaim: search.CoreClaim{
			ID:         claimID,
			Confidence: HighConfidence,
		},
		Prop: search.GetStandardPropertyReference("ALSO_KNOWN_AS"),
		HTML: search.TranslatableHTMLString{
			"en": escapedName,
		},
	}
	err := document.Add(claim)
	if err != nil {
		log.Error().Str("doc", string(document.ID)).Str("entity", id).Str("claim", string(claimID)).Str("title", title).
			Err(err).Fields(errors.AllDetails(err)).Msg("claim cannot be added")
	}
}
