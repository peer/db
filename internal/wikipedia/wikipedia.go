package wikipedia

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/es"
)

var (
	NameSpaceWikipediaFile = uuid.MustParse("94b1c372-bc28-454c-a45a-2e4d29d15146")

	WikimediaCommonsFileError = errors.Base("file is from Wikimedia Commons error")
)

func ConvertWikipediaImage(
	ctx context.Context, log zerolog.Logger, httpClient *retryablehttp.Client, token string, apiLimit int, image Image,
) (*search.Document, errors.E) {
	return convertImage(ctx, log, httpClient, NameSpaceWikipediaFile, "en", "en.wikipedia.org", "ENGLISH_WIKIPEDIA", token, apiLimit, image)
}

// TODO: Store the revision, license, and source used for the HTML into a meta claim.
// TODO: Investigate how to make use of additional entities metadata. See: https://www.mediawiki.org/wiki/Topic:Wotwu75akwx2wnsb
// TODO: Make internal links to other articles work in HTML (link to PeerDB documents instead).
// TODO: Remove links to other articles which do not exist, if there are any.
// TODO: Clean custom tags and attributes used in HTML to add metadata into HTML, potentially extract and store that. See: https://www.mediawiki.org/wiki/Specs/HTML/2.4.0
// TODO: Remove some templates (e.g., infobox, top-level notices) and convert them to claims.
// TODO: Extract all links pointing out of the article into claims and reverse claims (so if they point to other documents, they should have backlink as claim).
func ConvertWikipediaArticle(id, html string, document *search.Document) errors.E {
	body, doc, err := ExtractArticle(html)
	if err != nil {
		errE := errors.WithMessage(err, "article extraction failed")
		errors.Details(errE)["doc"] = document.ID.String()
		return errE
	}

	claimID := search.GetID(NameSpaceWikidata, id, "ARTICLE", 0)
	err = updateTextClaim(claimID, document, "ARTICLE", body)
	if err != nil {
		return err
	}

	claimID = search.GetID(NameSpaceWikidata, id, "LABEL", 0, "HAS_ARTICLE", 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim == nil {
		claim := &search.RelationClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop: search.GetCorePropertyReference("LABEL"),
			To:   search.GetCorePropertyReference("HAS_ARTICLE"),
		}
		err = document.Add(claim)
		if err != nil {
			errE := errors.WithMessage(err, "claim cannot be added")
			errors.Details(errE)["doc"] = document.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			return errE
		}
	}

	summary, err := ExtractArticleSummary(doc)
	if err != nil {
		errE := errors.WithMessage(err, "summary extraction failed")
		errors.Details(errE)["doc"] = document.ID.String()
		return errE
	}

	// TODO: Remove summary if is now empty, but before it was not.
	if summary != "" {
		err := updateDescription(NameSpaceWikidata, id, "ARTICLE", 0, summary, document)
		if err != nil {
			return err
		}
	}

	return nil
}

func ConvertFileDescription(namespace uuid.UUID, id, from, html string, document *search.Document) errors.E {
	descriptions, err := ExtractFileDescriptions(html)
	if err != nil {
		errE := errors.WithMessage(err, "descriptions extraction failed")
		errors.Details(errE)["doc"] = document.ID.String()
		return errE
	}

	// TODO: Remove old descriptions if there are now less of them then before.
	for i, description := range descriptions {
		err := updateDescription(namespace, id, from, i, description, document)
		if err != nil {
			return err
		}
	}

	return nil
}

func ConvertCategoryDescription(id, from, html string, document *search.Document) errors.E {
	return convertDescription(NameSpaceWikidata, id, from, html, document, ExtractCategoryDescription)
}

func updateTextClaim(claimID identifier.Identifier, document *search.Document, prop, value string) errors.E {
	existingClaim := document.GetByID(claimID)
	if existingClaim != nil {
		claim, ok := existingClaim.(*search.TextClaim)
		if !ok {
			errE := errors.New("unexpected claim type")
			errors.Details(errE)["doc"] = document.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.TextClaim{})
			return errE
		}
		claim.HTML["en"] = value
	} else {
		claim := &search.TextClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop: search.GetCorePropertyReference(prop),
			HTML: search.TranslatableHTMLString{
				"en": value,
			},
		}
		err := document.Add(claim)
		if err != nil {
			errE := errors.WithMessage(err, "claim cannot be added")
			errors.Details(errE)["doc"] = document.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			return errE
		}
	}

	return nil
}

func updateDescription(namespace uuid.UUID, id, from string, i int, description string, document *search.Document) errors.E {
	// A slightly different construction for claimID so that it does not overlap with any other descriptions.
	claimID := search.GetID(namespace, id, from, 0, "DESCRIPTION", i)
	return updateTextClaim(claimID, document, "DESCRIPTION", description)
}

func convertDescription(namespace uuid.UUID, id, from, html string, document *search.Document, extract func(string) (string, errors.E)) errors.E {
	description, err := extract(html)
	if err != nil {
		errE := errors.WithMessage(err, "description extraction failed")
		errors.Details(errE)["doc"] = document.ID.String()
		return errE
	}

	// TODO: Remove description if is now empty, but before it was not.
	if description != "" {
		err := updateDescription(namespace, id, from, 0, description, document)
		if err != nil {
			return err
		}
	}

	return nil
}

func SetPageID(namespace uuid.UUID, mnemonicPrefix string, id string, pageID int64, document *search.Document) errors.E {
	claimID := search.GetID(namespace, id, mnemonicPrefix+"_PAGE_ID", 0)
	existingClaim := document.GetByID(claimID)

	if existingClaim != nil {
		claim, ok := existingClaim.(*search.IdentifierClaim)
		if !ok {
			errE := errors.New("unexpected page id claim type")
			errors.Details(errE)["doc"] = document.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", &search.IdentifierClaim{})
			return errE
		}
		claim.Identifier = strconv.FormatInt(pageID, 10)
	} else {
		claim := &search.IdentifierClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop:       search.GetCorePropertyReference(mnemonicPrefix + "_PAGE_ID"),
			Identifier: strconv.FormatInt(pageID, 10),
		}
		err := document.Add(claim)
		if err != nil {
			errE := errors.WithMessage(err, "claim cannot be added")
			errors.Details(errE)["doc"] = document.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			return errE
		}
	}

	return nil
}

func ConvertTemplateDescription(id, from string, html string, document *search.Document) errors.E {
	return convertDescription(NameSpaceWikidata, id, from, html, document, ExtractTemplateDescription)
}

func GetWikipediaFile(ctx context.Context, index string, esClient *elastic.Client, name string) (*search.Document, *elastic.SearchHit, errors.E) {
	document, hit, errE := getDocumentFromESByProp(ctx, index, esClient, "ENGLISH_WIKIPEDIA_FILE_NAME", name)
	if errors.Is(errE, NotFoundError) {
		// Passthrough.
	} else if errE != nil {
		errors.Details(errE)["file"] = name
		return nil, nil, errE
	} else {
		return document, hit, nil
	}

	// Is there a Wikimedia Commons file under that name? Most files with article on Wikipedia
	// are in fact from Wikimedia Commons and have the same name. There can also be files on Wikipedia
	// from Wikimedia Commons which have different names so this can have false negatives.
	// False positives might also be possible but are probably harmless: we already did not
	// find a Wikipedia file, so we are primarily trying to understand why not.
	_, _, errE2 := getDocumentFromESByProp(ctx, index, esClient, "WIKIMEDIA_COMMONS_FILE_NAME", name)
	if errors.Is(errE2, NotFoundError) {
		// We have not found a Wikimedia Commons file. Return the original error.
		errors.Details(errE)["file"] = name
		return nil, nil, errE
	} else if errE2 != nil {
		errors.Details(errE2)["file"] = name
		return nil, nil, errors.WithMessage(errE2, "checking for Wikimedia Commons")
	}

	// We found a Wikimedia Commons file.
	errE = errors.WithStack(WikimediaCommonsFileError)
	errors.Details(errE)["file"] = name
	errors.Details(errE)["url"] = fmt.Sprintf("https://commons.wikimedia.org/wiki/File:%s", name)
	return nil, nil, errE
}

// TODO: How to remove categories which has previously been added but are later on removed?
func ConvertArticleInCategories(log zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id string, article mediawiki.Article, document *search.Document) errors.E {
	for _, category := range article.Categories {
		convertInCategory(log, namespace, mnemonicPrefix, id, article.Name, category.Name, document)
	}
	return nil
}

// TODO: How to remove templates which has previously been added but are later on removed?
func ConvertArticleUsedTemplates(log zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id string, article mediawiki.Article, document *search.Document) errors.E {
	for _, template := range article.Templates {
		convertUsedTemplate(log, namespace, mnemonicPrefix, id, article.Name, template.Name, document)
	}
	return nil
}

// TODO: How to remove categories which has previously been added but are later on removed?
func ConvertPageInCategories(log zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id string, page AllPagesPage, document *search.Document) errors.E {
	for _, category := range page.Categories {
		convertInCategory(log, namespace, mnemonicPrefix, id, page.Title, category.Title, document)
	}
	return nil
}

// TODO: How to remove templates which has previously been added but are later on removed?
func ConvertPageUsedTemplates(log zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id string, page AllPagesPage, document *search.Document) errors.E {
	for _, template := range page.Templates {
		convertUsedTemplate(log, namespace, mnemonicPrefix, id, page.Title, template.Title, document)
	}
	return nil
}

func convertInCategory(log zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id, title, category string, document *search.Document) {
	if !strings.HasPrefix(category, "Category:") {
		return
	}

	claimID := search.GetID(namespace, id, "IN_"+mnemonicPrefix+"_CATEGORY", 0, category, 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim == nil {
		claim := &search.RelationClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop: search.GetCorePropertyReference("IN_" + mnemonicPrefix + "_CATEGORY"),
			To:   getDocumentReference(category, mnemonicPrefix),
		}
		err := document.Add(claim)
		if err != nil {
			log.Error().Str("doc", document.ID.String()).Str("entity", id).Str("claim", claimID.String()).Str("title", title).
				Err(err).Fields(errors.AllDetails(err)).Msg("claim cannot be added")
		}
	}
}

func convertUsedTemplate(log zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id, title, template string, document *search.Document) {
	if !strings.HasPrefix(template, "Template:") && !strings.HasPrefix(template, "Module:") {
		return
	}

	claimID := search.GetID(namespace, id, "USES_"+mnemonicPrefix+"_TEMPLATE", template, 0)
	existingClaim := document.GetByID(claimID)
	if existingClaim == nil {
		claim := &search.RelationClaim{
			CoreClaim: search.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop: search.GetCorePropertyReference("USES_" + mnemonicPrefix + "_TEMPLATE"),
			To:   getDocumentReference(template, mnemonicPrefix),
		}
		err := document.Add(claim)
		if err != nil {
			log.Error().Str("doc", document.ID.String()).Str("entity", id).Str("claim", claimID.String()).Str("title", title).
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
	claimID := search.GetID(namespace, id, "NAME", redirect)
	existingClaim := document.GetByID(claimID)
	if existingClaim != nil {
		return
	}
	escapedName := html.EscapeString(strings.ReplaceAll(redirect, "_", " "))
	found := false
	for _, claim := range document.Get(*search.GetCorePropertyID("NAME")) {
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
			Confidence: es.MediumConfidence,
		},
		Prop: search.GetCorePropertyReference("NAME"),
		HTML: search.TranslatableHTMLString{
			"en": escapedName,
		},
	}
	err := document.Add(claim)
	if err != nil {
		log.Error().Str("doc", document.ID.String()).Str("entity", id).Str("claim", claimID.String()).Str("title", title).
			Err(err).Fields(errors.AllDetails(err)).Msg("claim cannot be added")
	}
}
