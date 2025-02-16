package main

import (
	"context"
	"encoding/json"
	"fmt"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb/document"
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

type NaturetaProduct struct {
	Name        string                  `pagser:".product-single .product-cover h1->text()"`
	Details     []NaturetaProductDetail `pagser:".product-single .product-cover .infos .info"`
	Description string                  `pagser:".product-single .product-cover .description->text()"`
	Category    string                  `pagser:".product-single .product-cover .category->text()"`
	Ingredients string                  `pagser:".product-single .ingredients-text .ingredients-text--wrapper p->text()"`
}

func (n Natureta) Run(
	ctx context.Context, config *Config, httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	indexingCount, indexingSize *x.Counter,
) errors.E {
	if n.Disabled {
		return nil
	}

	naturetaMain, errE := indexer.GetWebData[NaturetaProducts](ctx, httpClient, mainNaturetaURL)
	if errE != nil {
		return errE
	}

	products := mapset.NewThreadUnsafeSet[string]()
	products.Append(naturetaMain.Products...)

	for _, categoryURL := range naturetaMain.Categories {
		// We already processed the main category.
		if categoryURL == mainNaturetaURL {
			continue
		}

		naturetaProducts, errE := indexer.GetWebData[NaturetaProducts](ctx, httpClient, categoryURL)
		if errE != nil {
			return errE
		}

		products.Append(naturetaProducts.Products...)
	}

	// TODO: Use Go iterators once supported. See: https://github.com/deckarep/golang-set/issues/141
	for _, productURL := range products.ToSlice() {
		naturetaProduct, errE := indexer.GetWebData[NaturetaProduct](ctx, httpClient, productURL)
		if errE != nil {
			return errE
		}

		for _, detail := range naturetaProduct.Details {
			if detail.Name == "" {
				continue
			}
		}
	}

	return nil
}
