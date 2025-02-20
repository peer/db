package main

import (
	"context"
	"encoding/json"
	"html"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/indexer"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	mainNaturetaURL = "https://natureta.si/trgovina/10-najljubsih/"
)

type Natureta struct {
	Disabled bool `default:"false" help:"Do not import Natureta data. Default: false."`
}

type NaturetaProducts struct {
	Categories []string `pagser:".sidebar--wrapper .menu a->eachAttr(href)"`
	Products   []string `pagser:".products .product--wrapper>a->eachAttr(href)"`
}

type NaturetaProductDetail struct {
	Name  string `pagser:".text->text()"`
	Value string `pagser:".detail->text()"`
}

// TODO: Use HTML for description (product pages have some limited HTML in descriptions) and sanitize it.

type NaturetaProduct struct {
	Name        string                  `pagser:".product-single .product-cover h1->text()"`
	Details     []NaturetaProductDetail `pagser:".product-single .product-cover .infos .info"`
	Description string                  `pagser:".product-single .product-cover .description->text()"`
	Category    string                  `pagser:".product-single .product-cover .category->text()"`
	Ingredients string                  `pagser:".product-single .ingredients-text .ingredients-text--wrapper p->text()"`
}

func makeNaturetaDoc(product NaturetaProduct, productURL string) (document.D, errors.E) {
	name := strings.TrimSpace(product.Name)
	if name == "" {
		return document.D{}, errors.New("empty name")
	}

	doc := document.D{ //nolint:dupl
		CoreDocument: document.CoreDocument{
			// TODO: Use some better ID for these products and not URL.
			ID:    document.GetID(NameSpaceProducts, "NATURETA", productURL),
			Score: document.LowConfidence,
		},
		Claims: &document.ClaimTypes{
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "NATURETA", productURL, "TYPE", 0, "BRANDED_FOOD", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("BRANDED_FOOD"),
				},
			},
			Text: document.TextClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "NATURETA", productURL, "NAME", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("NAME"),
					HTML: document.TranslatableHTMLString{
						// TODO: Flag as Slovenian language.
						"en": html.EscapeString(name),
					},
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "NATURETA", productURL, "BRAND_OWNER", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("BRAND_OWNER"),
					// TODO: Flag as Slovenian language.
					HTML: document.TranslatableHTMLString{"en": html.EscapeString("Natureta")},
				},
			},
		},
	}

	if s := strings.TrimSpace(product.Description); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "NATURETA", productURL, "DESCRIPTION", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("DESCRIPTION"),
			// TODO: Flag as Slovenian language.
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(product.Ingredients); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "NATURETA", productURL, "INGREDIENTS", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("INGREDIENTS"),
			// TODO: Flag as Slovenian language.
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(product.Category); s != "" {
		// TODO: Should this be a text claim? Or a relation claim.
		errE := doc.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "NATURETA", productURL, "CATEGORY", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("CATEGORY"),
			String: s,
		})
		if errE != nil {
			return doc, errE
		}
	}

	for _, detail := range product.Details {
		if s := strings.TrimSpace(detail.Value); s != "" {
			switch detail.Name {
			case "":
				continue
			case "Gramatura:":
				// TODO: Parse into amount based claim.
				errE := doc.Add(&document.TextClaim{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "NATURETA", productURL, "PACKAGING_SIZE_DESCRIPTION", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("PACKAGING_SIZE_DESCRIPTION"),
					// TODO: Flag as Slovenian language.
					HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
				})
				if errE != nil {
					return doc, errE
				}
			case "Pakiranje:":
				// TODO: Parse into amount based claim.
				errE := doc.Add(&document.TextClaim{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "NATURETA", productURL, "PACKAGING_SIZE_DESCRIPTION", 1),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("PACKAGING_SIZE_DESCRIPTION"),
					// TODO: Flag as Slovenian language.
					HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
				})
				if errE != nil {
					return doc, errE
				}
			default:
				errE := errors.New("unsupported detail")
				errors.Details(errE)["detail"] = detail.Name
				return doc, errE
			}
		}
	}

	return doc, nil
}

func (n Natureta) Run(
	ctx context.Context, config *Config, httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	indexingCount, indexingSize *x.Counter,
) errors.E {
	if n.Disabled {
		return nil
	}

	naturetaMain, errE := indexer.GetWebData[NaturetaProducts](ctx, httpClient, mainNaturetaURL, indexer.ExtractData)
	if errE != nil {
		return errE
	}

	products := mapset.NewThreadUnsafeSet[string]()
	products.Append(naturetaMain.Products...)

	for _, categoryURL := range naturetaMain.Categories {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}

		// We already processed the main category.
		if categoryURL == mainNaturetaURL {
			continue
		}

		ps, errE := indexer.GetWebData[NaturetaProducts](ctx, httpClient, categoryURL, indexer.ExtractData)
		if errE != nil {
			return errE
		}

		products.Append(ps.Products...)
	}

	config.Logger.Info().Int("total", products.Cardinality()).Msg("retrieved Natureta data")

	description := "Natureta processing"
	progress := es.Progress(config.Logger, nil, nil, nil, description)
	indexingSize.Add(int64(products.Cardinality()))

	count := x.Counter(0)
	ticker := x.NewTicker(ctx, &count, x.NewCounter(int64(products.Cardinality())), indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	// TODO: Use Go iterators once supported. See: https://github.com/deckarep/golang-set/issues/141
	for i, productURL := range products.ToSlice() {
		if ctx.Err() != nil {
			return errors.WithStack(ctx.Err())
		}
		config.Logger.Debug().
			Int("index", i).
			Str("url", productURL).
			Msg("processing Natureta record")

		product, errE := indexer.GetWebData[NaturetaProduct](ctx, httpClient, productURL, indexer.ExtractData)
		if errE != nil {
			return errE
		}

		doc, errE := makeNaturetaDoc(product, productURL)
		if errE != nil {
			errors.Details(errE)["url"] = productURL
			return errE
		}

		count.Increment()
		indexingCount.Increment()

		config.Logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE = peerdb.InsertOrReplaceDocument(ctx, store, &doc)
		if errE != nil {
			errors.Details(errE)["url"] = productURL
			return errE
		}
	}

	config.Logger.Info().
		Int64("count", count.Count()).
		Int("total", products.Cardinality()).
		Msg(description + " done")

	return nil
}
