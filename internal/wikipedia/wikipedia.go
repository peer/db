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

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
)

var (
	//nolint:gochecknoglobals
	NameSpaceWikipediaFile = uuid.MustParse("94b1c372-bc28-454c-a45a-2e4d29d15146")

	ErrWikimediaCommonsFile = errors.Base("file is from Wikimedia Commons error")
)

func ConvertWikipediaImage(
	ctx context.Context, logger zerolog.Logger, httpClient *retryablehttp.Client, token string, apiLimit int, image Image,
) (*peerdb.Document, errors.E) {
	return convertImage(ctx, logger, httpClient, NameSpaceWikipediaFile, "en", "en.wikipedia.org", "ENGLISH_WIKIPEDIA", token, apiLimit, image)
}

// TODO: Store the revision, license, and source used for the HTML into a meta claim.
// TODO: Investigate how to make use of additional entities metadata. See: https://www.mediawiki.org/wiki/Topic:Wotwu75akwx2wnsb
// TODO: Make internal links to other articles work in HTML (link to PeerDB documents instead).
// TODO: Remove links to other articles which do not exist, if there are any.
// TODO: Clean custom tags and attributes used in HTML to add metadata into HTML, potentially extract and store that. See: https://www.mediawiki.org/wiki/Specs/HTML/2.4.0
// TODO: Remove some templates (e.g., infobox, top-level notices) and convert them to claims.
// TODO: Extract all links pointing out of the article into claims and reverse claims (so if they point to other documents, they should have backlink as claim).
func ConvertWikipediaArticle(id, html string, doc *peerdb.Document) errors.E {
	body, article, err := ExtractArticle(html)
	if err != nil {
		errE := errors.WithMessage(err, "article extraction failed")
		errors.Details(errE)["doc"] = doc.ID.String()
		return errE
	}

	claimID := peerdb.GetID(NameSpaceWikidata, id, "ARTICLE", 0)
	err = updateTextClaim(claimID, doc, "ARTICLE", body)
	if err != nil {
		return err
	}

	claimID = peerdb.GetID(NameSpaceWikidata, id, "LABEL", 0, "HAS_ARTICLE", 0)
	existingClaim := doc.GetByID(claimID)
	if existingClaim == nil {
		claim := &document.RelationClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop: peerdb.GetCorePropertyReference("LABEL"),
			To:   peerdb.GetCorePropertyReference("HAS_ARTICLE"),
		}
		err = doc.Add(claim)
		if err != nil {
			errE := errors.WithMessage(err, "claim cannot be added")
			errors.Details(errE)["doc"] = doc.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			return errE
		}
	}

	summary, err := ExtractArticleSummary(article)
	if err != nil {
		errE := errors.WithMessage(err, "summary extraction failed")
		errors.Details(errE)["doc"] = doc.ID.String()
		return errE
	}

	// TODO: Remove summary if is now empty, but before it was not.
	if summary != "" {
		err := updateDescription(NameSpaceWikidata, id, "ARTICLE", 0, summary, doc)
		if err != nil {
			return err
		}
	}

	return nil
}

func ConvertFileDescription(namespace uuid.UUID, id, from, html string, doc *peerdb.Document) errors.E {
	descriptions, err := ExtractFileDescriptions(html)
	if err != nil {
		errE := errors.WithMessage(err, "descriptions extraction failed")
		errors.Details(errE)["doc"] = doc.ID.String()
		return errE
	}

	// TODO: Remove old descriptions if there are now less of them then before.
	for i, description := range descriptions {
		err := updateDescription(namespace, id, from, i, description, doc)
		if err != nil {
			return err
		}
	}

	return nil
}

func ConvertCategoryDescription(id, from, html string, doc *peerdb.Document) errors.E {
	return convertDescription(NameSpaceWikidata, id, from, html, doc, ExtractCategoryDescription)
}

func updateTextClaim(claimID identifier.Identifier, doc *peerdb.Document, prop, value string) errors.E {
	existingClaim := doc.GetByID(claimID)
	if existingClaim != nil {
		claim, ok := existingClaim.(*document.TextClaim)
		if !ok {
			errE := errors.New("unexpected claim type")
			errors.Details(errE)["doc"] = doc.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", new(document.TextClaim))
			return errE
		}
		claim.HTML["en"] = value
	} else {
		claim := &document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop: peerdb.GetCorePropertyReference(prop),
			HTML: document.TranslatableHTMLString{
				"en": value,
			},
		}
		err := doc.Add(claim)
		if err != nil {
			errE := errors.WithMessage(err, "claim cannot be added")
			errors.Details(errE)["doc"] = doc.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			return errE
		}
	}

	return nil
}

func updateDescription(namespace uuid.UUID, id, from string, i int, description string, doc *peerdb.Document) errors.E {
	// A slightly different construction for claimID so that it does not overlap with any other descriptions.
	claimID := peerdb.GetID(namespace, id, from, 0, "DESCRIPTION", i)
	return updateTextClaim(claimID, doc, "DESCRIPTION", description)
}

func convertDescription(namespace uuid.UUID, id, from, html string, doc *peerdb.Document, extract func(string) (string, errors.E)) errors.E {
	description, err := extract(html)
	if err != nil {
		errE := errors.WithMessage(err, "description extraction failed")
		errors.Details(errE)["doc"] = doc.ID.String()
		return errE
	}

	// TODO: Remove description if is now empty, but before it was not.
	if description != "" {
		err := updateDescription(namespace, id, from, 0, description, doc)
		if err != nil {
			return err
		}
	}

	return nil
}

func SetPageID(namespace uuid.UUID, mnemonicPrefix string, id string, pageID int64, doc *peerdb.Document) errors.E {
	claimID := peerdb.GetID(namespace, id, mnemonicPrefix+"_PAGE_ID", 0)
	existingClaim := doc.GetByID(claimID)

	if existingClaim != nil {
		claim, ok := existingClaim.(*document.IdentifierClaim)
		if !ok {
			errE := errors.New("unexpected page id claim type")
			errors.Details(errE)["doc"] = doc.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			errors.Details(errE)["got"] = fmt.Sprintf("%T", existingClaim)
			errors.Details(errE)["expected"] = fmt.Sprintf("%T", new(document.IdentifierClaim))
			return errE
		}
		claim.Identifier = strconv.FormatInt(pageID, 10)
	} else {
		claim := &document.IdentifierClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop:       peerdb.GetCorePropertyReference(mnemonicPrefix + "_PAGE_ID"),
			Identifier: strconv.FormatInt(pageID, 10),
		}
		err := doc.Add(claim)
		if err != nil {
			errE := errors.WithMessage(err, "claim cannot be added")
			errors.Details(errE)["doc"] = doc.ID.String()
			errors.Details(errE)["claim"] = claimID.String()
			return errE
		}
	}

	return nil
}

func ConvertTemplateDescription(id, from string, html string, doc *peerdb.Document) errors.E {
	return convertDescription(NameSpaceWikidata, id, from, html, doc, ExtractTemplateDescription)
}

func GetWikipediaFile(ctx context.Context, index string, esClient *elastic.Client, name string) (*peerdb.Document, *elastic.SearchHit, errors.E) {
	doc, hit, errE := getDocumentFromESByProp(ctx, index, esClient, "ENGLISH_WIKIPEDIA_FILE_NAME", name)
	if errors.Is(errE, ErrNotFound) { //nolint:revive
		// Passthrough.
	} else if errE != nil {
		errors.Details(errE)["file"] = name
		return nil, nil, errE
	} else {
		return doc, hit, nil
	}

	// Is there a Wikimedia Commons file under that name? Most files with article on Wikipedia
	// are in fact from Wikimedia Commons and have the same name. There can also be files on Wikipedia
	// from Wikimedia Commons which have different names so this can have false negatives.
	// False positives might also be possible but are probably harmless: we already did not
	// find a Wikipedia file, so we are primarily trying to understand why not.
	_, _, errE2 := getDocumentFromESByProp(ctx, index, esClient, "WIKIMEDIA_COMMONS_FILE_NAME", name)
	if errors.Is(errE2, ErrNotFound) {
		// We have not found a Wikimedia Commons file. Return the original error.
		errors.Details(errE)["file"] = name
		return nil, nil, errE
	} else if errE2 != nil {
		errors.Details(errE2)["file"] = name
		return nil, nil, errors.WithMessage(errE2, "checking for Wikimedia Commons")
	}

	// We found a Wikimedia Commons file.
	errE = errors.WithStack(ErrWikimediaCommonsFile)
	errors.Details(errE)["file"] = name
	errors.Details(errE)["url"] = fmt.Sprintf("https://commons.wikimedia.org/wiki/File:%s", name)
	return nil, nil, errE
}

// TODO: How to remove categories which has previously been added but are later on removed?
func ConvertArticleInCategories(logger zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id string, article mediawiki.Article, doc *peerdb.Document) errors.E {
	for _, category := range article.Categories {
		convertInCategory(logger, namespace, mnemonicPrefix, id, article.Name, category.Name, doc)
	}
	return nil
}

// TODO: How to remove templates which has previously been added but are later on removed?
func ConvertArticleUsedTemplates(logger zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id string, article mediawiki.Article, doc *peerdb.Document) errors.E {
	for _, template := range article.Templates {
		convertUsedTemplate(logger, namespace, mnemonicPrefix, id, article.Name, template.Name, doc)
	}
	return nil
}

// TODO: How to remove categories which has previously been added but are later on removed?
func ConvertPageInCategories(logger zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id string, page AllPagesPage, doc *peerdb.Document) errors.E {
	for _, category := range page.Categories {
		convertInCategory(logger, namespace, mnemonicPrefix, id, page.Title, category.Title, doc)
	}
	return nil
}

// TODO: How to remove templates which has previously been added but are later on removed?
func ConvertPageUsedTemplates(logger zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id string, page AllPagesPage, doc *peerdb.Document) errors.E {
	for _, template := range page.Templates {
		convertUsedTemplate(logger, namespace, mnemonicPrefix, id, page.Title, template.Title, doc)
	}
	return nil
}

func convertInCategory(logger zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id, title, category string, doc *peerdb.Document) {
	if !strings.HasPrefix(category, "Category:") {
		return
	}

	claimID := peerdb.GetID(namespace, id, "IN_"+mnemonicPrefix+"_CATEGORY", 0, category, 0)
	existingClaim := doc.GetByID(claimID)
	if existingClaim == nil {
		claim := &document.RelationClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop: peerdb.GetCorePropertyReference("IN_" + mnemonicPrefix + "_CATEGORY"),
			To:   getDocumentReference(category, mnemonicPrefix),
		}
		errE := doc.Add(claim)
		if errE != nil {
			logger.Error().Str("doc", doc.ID.String()).Str("entity", id).Str("claim", claimID.String()).Str("title", title).
				Err(errE).Msg("claim cannot be added")
		}
	}
}

func convertUsedTemplate(logger zerolog.Logger, namespace uuid.UUID, mnemonicPrefix, id, title, template string, doc *peerdb.Document) {
	if !strings.HasPrefix(template, "Template:") && !strings.HasPrefix(template, "Module:") {
		return
	}

	claimID := peerdb.GetID(namespace, id, "USES_"+mnemonicPrefix+"_TEMPLATE", template, 0)
	existingClaim := doc.GetByID(claimID)
	if existingClaim == nil {
		claim := &document.RelationClaim{
			CoreClaim: document.CoreClaim{
				ID:         claimID,
				Confidence: es.HighConfidence,
			},
			Prop: peerdb.GetCorePropertyReference("USES_" + mnemonicPrefix + "_TEMPLATE"),
			To:   getDocumentReference(template, mnemonicPrefix),
		}
		errE := doc.Add(claim)
		if errE != nil {
			logger.Error().Str("doc", doc.ID.String()).Str("entity", id).Str("claim", claimID.String()).Str("title", title).
				Err(errE).Msg("claim cannot be added")
		}
	}
}

// TODO: How to remove redirects which has previously been added but are later on removed?
func ConvertArticleRedirects(logger zerolog.Logger, namespace uuid.UUID, id string, article mediawiki.Article, doc *peerdb.Document) errors.E {
	for _, redirect := range article.Redirects {
		convertRedirect(logger, namespace, id, article.Name, redirect.Name, doc)
	}
	return nil
}

// TODO: How to remove redirects which has previously been added but are later on removed?
func ConvertPageRedirects(logger zerolog.Logger, namespace uuid.UUID, id string, page AllPagesPage, doc *peerdb.Document) errors.E {
	for _, redirect := range page.Redirects {
		convertRedirect(logger, namespace, id, page.Title, redirect.Title, doc)
	}
	return nil
}

func convertRedirect(logger zerolog.Logger, namespace uuid.UUID, id, title, redirect string, doc *peerdb.Document) {
	claimID := peerdb.GetID(namespace, id, "NAME", redirect)
	existingClaim := doc.GetByID(claimID)
	if existingClaim != nil {
		return
	}
	escapedName := html.EscapeString(strings.ReplaceAll(redirect, "_", " "))
	found := false
	for _, claim := range doc.Get(peerdb.GetCorePropertyID("NAME")) {
		if c, ok := claim.(*document.TextClaim); ok && c.HTML["en"] == escapedName {
			found = true
			break
		}
	}
	if found {
		return
	}
	claim := &document.TextClaim{
		CoreClaim: document.CoreClaim{
			ID:         claimID,
			Confidence: es.MediumConfidence,
		},
		Prop: peerdb.GetCorePropertyReference("NAME"),
		HTML: document.TranslatableHTMLString{
			"en": escapedName,
		},
	}
	errE := doc.Add(claim)
	if errE != nil {
		logger.Error().Str("doc", doc.ID.String()).Str("entity", id).Str("claim", claimID.String()).Str("title", title).
			Err(errE).Msg("claim cannot be added")
	}
}
