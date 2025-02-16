package main

import (
	"context"
	"encoding/json"
	"html"
	"io"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/hashicorp/go-retryablehttp"
	sitemap "github.com/oxffaa/gopher-parse-sitemap"
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
	zitoURL            = "https://www.zito.si"
	zitoProductsPrefix = "https://www.zito.si/sl/izdelek/"
)

type Zito struct {
	Disabled bool `default:"false" help:"Do not import Zito data. Default: false."`
}

type ZitoProductDetail struct {
	Type  string `pagser:"->class('atr-name-')"`
	Name  string `pagser:".title->text()"`
	Value string `pagser:".value->text()"`
}

type ZitoProductDetails struct {
	// TODO: Should have spaces between text fragments. See: https://github.com/PuerkitoBio/goquery/issues/443
	Description string              `pagser:".ui-tabs-content .w-value-text->text()"`
	Details     []ZitoProductDetail `pagser:".w-productAttributes li"`
	Title       string              `pagser:".w-rte h2->text()"`
}

type ZitoProduct struct {
	SchemaOrg indexer.SchemaOrg `pagser:"script[type='application/ld+json']->schemaOrg()"`
	Fragments string            `pagser:"script[type='text/html']->text()"`
}

func makeZitoDoc(product ZitoProduct, _ ZitoProductDetails) (document.D, errors.E) {
	name := strings.TrimSpace(product.SchemaOrg.Name)
	if name == "" {
		return document.D{}, errors.New("empty name")
	}

	id := strings.TrimSpace(product.SchemaOrg.ID)
	if id == "" {
		return document.D{}, errors.New("empty ID")
	}

	doc := document.D{ //nolint:dupl
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "ZITO", id),
			Score: document.LowConfidence,
		},
		Claims: &document.ClaimTypes{
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "ZITO", id, "TYPE", 0, "BRANDED_FOOD", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("BRANDED_FOOD"),
				},
			},
			Text: document.TextClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "ZITO", id, "NAME", 0),
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
						ID:         document.GetID(NameSpaceProducts, "ZITO", id, "BRAND_OWNER", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("BRAND_OWNER"),
					// TODO: Flag as Slovenian language.
					HTML: document.TranslatableHTMLString{"en": html.EscapeString("Žito")},
				},
			},
		},
	}

	if s := strings.TrimSpace(product.SchemaOrg.MPN); s != "" {
		errE := doc.Add(&document.IdentifierClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "ZITO", id, "GTIN", 0),
				Confidence: document.HighConfidence,
			},
			Prop:  document.GetCorePropertyReference("GTIN"),
			Value: s,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(product.SchemaOrg.Brand); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "ZITO", id, "BRAND_NAME", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("BRAND_NAME"),
			// TODO: Flag as Slovenian language.
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(product.SchemaOrg.Category); s != "" {
		// TODO: Should this be a text claim? Or a relation claim.
		errE := doc.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "ZITO", id, "CATEGORY", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("CATEGORY"),
			String: s,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(product.SchemaOrg.Description); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "ZITO", id, "DESCRIPTION", 0),
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

	// TODO: Use data from details.

	return doc, nil
}

func (n Zito) Run(
	ctx context.Context, config *Config, httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	indexingCount, indexingSize *x.Counter,
) errors.E {
	if n.Disabled {
		return nil
	}

	robots, errE := indexer.GetRobotsTxt(ctx, httpClient, zitoURL)
	if errE != nil {
		return errE
	}

	products := mapset.NewThreadUnsafeSet[string]()
	for _, sitemapURL := range robots.Sitemaps {
		ps, errE := indexer.GetWebData(ctx, httpClient, sitemapURL, func(in io.Reader) (mapset.Set[string], errors.E) {
			p := mapset.NewThreadUnsafeSet[string]()
			err := sitemap.Parse(in, func(entry sitemap.Entry) error {
				loc := entry.GetLocation()
				if strings.HasPrefix(loc, zitoProductsPrefix) {
					p.Add(loc)
				}
				return nil
			})
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return p, nil
		})
		if errE != nil {
			return errE
		}
		products = products.Union(ps)
	}

	description := "Zito processing"
	progress := es.Progress(config.Logger, nil, nil, nil, description)
	indexingSize.Add(int64(products.Cardinality()))

	count := x.Counter(0)
	size := x.NewCounter(int64(products.Cardinality()))
	ticker := x.NewTicker(ctx, &count, size, indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	// TODO: Use Go iterators once supported. See: https://github.com/deckarep/golang-set/issues/141
	for _, productURL := range products.ToSlice() {
		if ctx.Err() != nil {
			break
		}

		product, errE := indexer.GetWebData[ZitoProduct](ctx, httpClient, productURL, indexer.ExtractData)
		if errE != nil {
			return errE
		}

		// Pages have HTML fragments they inject with JavaScript into the page.
		productDetails, errE := indexer.ExtractData[ZitoProductDetails](strings.NewReader(product.Fragments))
		if errE != nil {
			return errE
		}

		if productDetails.Title == "Recept" {
			size.Add(-1)
			indexingSize.Add(-1)
			continue
		} else if productDetails.Title != "Opis izdelka" {
			errE = errors.New("unexpected page title")
			errors.Details(errE)["title"] = productDetails.Title
			errors.Details(errE)["url"] = productURL
			return errE
		}
		if product.SchemaOrg.Type != "Product" {
			errE = errors.New("unexpected type")
			errors.Details(errE)["type"] = product.SchemaOrg.Type
			errors.Details(errE)["url"] = productURL
		}

		doc, errE := makeZitoDoc(product, productDetails)
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
