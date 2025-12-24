package main

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"html"
	"io"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/krolaw/zipstream"
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
	fursDEJURL = "http://www.datoteke.fu.gov.si/DURS_zavezanci_DEJ.zip"
)

type FURSDEJ struct {
	Disabled bool `default:"false" help:"Do not import FURS DEJ data."`
}

type FursEntry struct {
	VATNumber          string `json:"idVatNo"`
	RegistrationNumber string `json:"idRegNo"`
	SKD                string `json:"skd"`
	Name               string `json:"company"`
	Address            string `json:"address"`
	FinancialOffice    string `json:"financialOffice"`
}

func makeFursDoc(furs FursEntry) (document.D, errors.E) {
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "FURS", furs.RegistrationNumber),
			Score: document.LowConfidence,
		},
		Claims: &document.ClaimTypes{
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "FURS", furs.RegistrationNumber, "COMPANY_REGISTRATION_NUMBER", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("COMPANY_REGISTRATION_NUMBER"),
					Value: furs.RegistrationNumber,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "FURS", furs.RegistrationNumber, "VAT_NUMBER", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("VAT_NUMBER"),
					Value: furs.VATNumber,
				},
			},
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "FURS", furs.Name, "TYPE", 0, "NAME", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("COMPANY"),
				},
			},
			Text: document.TextClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "FURS", furs.RegistrationNumber, "NAME", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("NAME"),
					HTML: document.TranslatableHTMLString{"en": html.EscapeString(furs.Name)},
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "FURS", furs.RegistrationNumber, "ADDRESS", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("ADDRESS"),
					HTML: document.TranslatableHTMLString{"en": html.EscapeString(furs.Address)},
				},
			},
			String: document.StringClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "FURS", furs.RegistrationNumber, "FINANCIAL_OFFICE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("FINANCIAL_OFFICE"),
					String: furs.FinancialOffice,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "FURS", furs.RegistrationNumber, "COUNTRY_OF_INCORPORATION", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("COUNTRY_OF_INCORPORATION"),
					String: "Slovenia",
				},
			},
		},
	}

	var errE errors.E
	if s := strings.TrimSpace(furs.SKD); s != "" {
		errE = doc.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "FURS", furs.RegistrationNumber, "SKD_2025", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("SKD_2025"),
			String: s,
		})
		if errE != nil {
			return doc, errE
		}
	}
	return doc, nil
}

//nolint:dupl
func (d FURSDEJ) Run(
	ctx context.Context,
	config *Config,
	httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	indexingCount, indexingSize *x.Counter,
) errors.E {
	if d.Disabled {
		return nil
	}

	records, errE := downloadFurs(ctx, httpClient, config.Logger, config.CacheDir, fursDEJURL)
	if errE != nil {
		return errE
	}

	config.Logger.Info().Int("total", len(records)).Msg("retrieved FURS DEJ data")

	description := "FURS DEJ processing"
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
		err := ctx.Err()
		if err != nil { // Check if context is canceled.
			return errors.WithStack(err)
		}
		config.Logger.Debug().
			Int("index", i).
			Str("id", record.RegistrationNumber).
			Msg("processing FURS DEJ record")

		doc, errE := makeFursDoc(record)

		if errE != nil {
			errors.Details(errE)["id"] = record.RegistrationNumber
			return errE
		}

		count.Increment()
		indexingCount.Increment()

		config.Logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE = peerdb.InsertOrReplaceDocument(ctx, store, &doc)
		if errE != nil {
			errors.Details(errE)["id"] = record.RegistrationNumber
			return errE
		}
	}

	config.Logger.Info().
		Int64("count", count.Count()).
		Int("total", len(records)).
		Msg(description + " done")

	return nil
}

func downloadFurs(ctx context.Context, httpClient *retryablehttp.Client, logger zerolog.Logger, cacheDir, url string) ([]FursEntry, errors.E) {
	reader, _, errE := indexer.CachedDownload(ctx, httpClient, logger, cacheDir, url)
	if errE != nil {
		return nil, errE
	}
	defer reader.Close() //nolint:errcheck

	zipReader := zipstream.NewReader(reader)
	var file *zip.FileHeader
	var err error
	for file, err = zipReader.Next(); err == nil; file, err = zipReader.Next() {
		if file.Name == "DURS_zavezanci_DEJ.txt" {
			records, errE := processFursDejFile(zipReader)
			if errE != nil {
				return nil, errE
			}
			return records, nil
		}
	}

	if errors.Is(err, io.EOF) {
		return nil, errors.New(`"DURS_zavezanci_DEJ.txt not found in ZIP"`)
	}

	return nil, errors.WithStack(err)
}

// trimAndExtract extracts a substring from a fixed-width text line.
func trimAndExtract(line string, start, end int) string {
	if len(line) < end {
		return "" // Prevent out-of-bounds errors.
	}
	return strings.TrimSpace(line[start:end])
}

// processFursDejFile reads and processes the in-memory file from ZIP.
func processFursDejFile(reader io.Reader) ([]FursEntry, errors.E) {
	scanner := bufio.NewScanner(reader)
	var records []FursEntry

	for scanner.Scan() {
		line := scanner.Text()

		zero := 0
		firstCol := 8
		secondCol := 19
		thirdCol := 26
		fourthCol := 127
		fifthCol := 241
		// Extract fields based on fixed positions.
		col1 := trimAndExtract(line, zero, firstCol)
		col2 := trimAndExtract(line, (firstCol + 1), secondCol)
		col3 := trimAndExtract(line, (secondCol + 1), thirdCol)
		col4 := trimAndExtract(line, (thirdCol + 1), fourthCol)
		col5 := trimAndExtract(line, (fourthCol + 1), fifthCol)
		col6 := line[len(line)-2:]

		// Append valid record.
		records = append(records, FursEntry{
			VATNumber:          col1,
			RegistrationNumber: col2,
			SKD:                col3,
			Name:               col4,
			Address:            col5,
			FinancialOffice:    col6,
		})
	}

	err := scanner.Err()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return records, nil
}
