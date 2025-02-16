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
	mainLjubljanskeSitemapURL = "https://www.l-m.si/sitemap.xml"
)

type LjubljanskeMlekarne struct {
	Disabled bool `default:"false" help:"Do not import Ljubljanske mlekarne data. Default: false."`
}

type LjubljanskeMlekarneProduct struct {
	Brand       string `pagser:".product .product-brand->text()"`
	Title       string `pagser:".product .product-title->text()"`
	Description string `pagser:".product .product-description->text()"`
	Ingredients string `pagser:".product .product-ingredients->text()"`
}

func makeLjubljanskeMlekarneDoc(product LjubljanskeMlekarneProduct, productURL string) (document.D, errors.E) {
	doc := document.D{
		CoreDocument: document.CoreDocument{
			// TODO: Use some better ID for these products and not URL.
			ID:    document.GetID(NameSpaceProducts, "LJUBLJANSKE_MLEKARNE", productURL),
			Score: document.LowConfidence,
		},
		Claims: &document.ClaimTypes{
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "LJUBLJANSKE_MLEKARNE", productURL, "TYPE", 0, "BRANDED_FOOD", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("BRANDED_FOOD"),
				},
			},
			Text: document.TextClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "LJUBLJANSKE_MLEKARNE", productURL, "NAME", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("NAME"),
					HTML: document.TranslatableHTMLString{
						// TODO: Flag as Slovenian language.
						"en": html.EscapeString(product.Title),
					},
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "LJUBLJANSKE_MLEKARNE", productURL, "BRAND_OWNER", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("BRAND_OWNER"),
					// TODO: Flag as Slovenian language.
					HTML: document.TranslatableHTMLString{"en": html.EscapeString("Ljubljanske mlekarne")},
				},
			},
		},
	}

	if s := strings.TrimSpace(product.Description); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "LJUBLJANSKE_MLEKARNE", productURL, "DESCRIPTION", 0),
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

	if s := strings.TrimSpace(product.Brand); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "LJUBLJANSKE_MLEKARNE", productURL, "BRAND_NAME", 0),
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

	if s := strings.TrimSpace(product.Ingredients); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "LJUBLJANSKE_MLEKARNE", productURL, "INGREDIENTS", 0),
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

	return doc, nil
}

func (n LjubljanskeMlekarne) Run(
	ctx context.Context, config *Config, httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	indexingCount, indexingSize *x.Counter,
) errors.E {
	if n.Disabled {
		return nil
	}

	products, errE := indexer.GetWebData[mapset.Set[string]](ctx, httpClient, mainLjubljanskeSitemapURL, func(in io.Reader) (mapset.Set[string], errors.E) {
		ps := mapset.NewThreadUnsafeSet[string]()
		err := sitemap.Parse(in, func(entry sitemap.Entry) error {
			ps.Add(entry.GetLocation())
			return nil
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return ps, nil
	})
	if errE != nil {
		return errE
	}

	description := "Ljubljanske mlekarne processing"
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
	for _, productURL := range products.ToSlice() {
		if ctx.Err() != nil {
			break
		}

		product, errE := indexer.GetWebData[LjubljanskeMlekarneProduct](ctx, httpClient, productURL, indexer.ExtractData)
		if errE != nil {
			return errE
		}

		doc, errE := makeLjubljanskeMlekarneDoc(product, productURL)
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
