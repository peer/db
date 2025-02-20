package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"html"
	"io"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
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
	datakickURL = "https://gtinsearch.org/items.csv"
)

type Datakick struct {
	Disabled bool `default:"false" help:"Do not import Datakick data. Default: false."`
}

type DatakickEntry struct {
	ID            string
	GTIN14        string
	BrandName     string
	Name          string
	PackagingSize string
	Ingredients   string
}

func getDatakick(ctx context.Context, httpClient *retryablehttp.Client, logger zerolog.Logger, cacheDir, url string) ([]DatakickEntry, errors.E) {
	reader, _, errE := indexer.CachedDownload(ctx, httpClient, logger, cacheDir, url)
	if errE != nil {
		return nil, errE
	}
	defer reader.Close()

	records, errE := processDatakickFile(reader)
	if errE != nil {
		return nil, errE
	}
	return records, nil
}

func processDatakickFile(reader io.ReadCloser) ([]DatakickEntry, errors.E) {
	csvReader := csv.NewReader(reader)

	var records []DatakickEntry
	for i := 0; ; i++ {
		row, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			errE := errors.WithMessage(err, "error reading CSV row")
			errors.Details(errE)["i"] = i
			return nil, errE
		}

		entry := DatakickEntry{
			ID:            row[0],
			GTIN14:        row[2],
			BrandName:     row[3],
			Name:          row[4],
			PackagingSize: row[5],
			Ingredients:   row[6],
		}

		records = append(records, entry)
	}
	return records, nil
}

func makeDatakickDoc(datakick DatakickEntry) (document.D, errors.E) {
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "DATAKICK", datakick.GTIN14),
			Score: document.LowConfidence,
		},
		Claims: &document.ClaimTypes{
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "DATAKICK", datakick.GTIN14, "GTIN", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("GTIN"),
					Value: datakick.GTIN14,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "DATAKICK", datakick.GTIN14, "DATAKICK_ID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("DATAKICK_ID"),
					Value: datakick.ID,
				},
			},
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "DATAKICK", datakick.Name, "TYPE", 0, "ITEM", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("ITEM"),
				},
			},
			Text: document.TextClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "DATAKICK", datakick.GTIN14, "NAME", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("NAME"),
					HTML: document.TranslatableHTMLString{"en": html.EscapeString(datakick.Name)},
				},
			},
		},
	}

	if s := strings.TrimSpace(datakick.BrandName); s != "" {
		// TODO: Should this be a text claim? Or a relation claim.
		errE := doc.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "DATAKICK", datakick.GTIN14, "BRAND_NAME", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("BRAND_NAME"),
			String: s,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(datakick.PackagingSize); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "DATAKICK", datakick.GTIN14, "PACKAGING_SIZE_DESCRIPTION", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("PACKAGING_SIZE_DESCRIPTION"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	if s := strings.TrimSpace(datakick.Ingredients); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "DATAKICK", datakick.GTIN14, "INGREDIENTS", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("INGREDIENTS"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(s)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	return doc, nil
}

//nolint:dupl
func (g Datakick) Run(
	ctx context.Context,
	config *Config,
	httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	indexingCount, indexingSize *x.Counter,
) errors.E {
	if g.Disabled {
		return nil
	}

	records, errE := getDatakick(ctx, httpClient, config.Logger, config.CacheDir, datakickURL)
	if errE != nil {
		return errE
	}

	config.Logger.Info().Int("total", len(records)).Msg("retrieved Datakick data")

	description := "Datakick processing"
	progress := es.Progress(config.Logger, nil, nil, nil, description)
	indexingSize.Add(int64(len(records)))

	count := x.Counter(0)
	ticker := x.NewTicker(ctx, &count, x.NewCounter(int64(len(records))), indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	for i, record := range records {
		if err := ctx.Err(); err != nil {
			return errors.WithStack(err)
		}
		config.Logger.Debug().
			Int("index", i).
			Str("id", record.GTIN14).
			Msg("processing Datakick record")

		doc, errE := makeDatakickDoc(record)
		if errE != nil {
			errors.Details(errE)["id"] = record.GTIN14
			return errE
		}

		count.Increment()
		indexingCount.Increment()

		config.Logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE = peerdb.InsertOrReplaceDocument(ctx, store, &doc)
		if errE != nil {
			errors.Details(errE)["id"] = record.GTIN14
			return errE
		}
	}

	config.Logger.Info().
		Int64("count", count.Count()).
		Int("total", len(records)).
		Msg(description + " done")

	return nil
}
