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
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/indexer"
	"gitlab.com/peerdb/peerdb/internal/types"
	"gitlab.com/peerdb/peerdb/store"
)

const (
	prsURL = "https://podatki.gov.si/dataset/9ee1a9aa-c224-4995-b2ad-3760d7af0748/resource/beb70929-3d0d-41c6-9af2-25d525d906d3/download/opsiprs.csv"
)

const (
	columnsCount             = 12
	registrationNumberLength = 10
)

type PRS struct {
	Disabled bool `default:"false" help:"Do not import PRS data."`
}

type PRSEntry struct {
	RegistrationNumber    string
	Name                  string
	HSEID                 string
	CompanyLegalForm      string
	RegistrationAuthority string
	Street                string
	HouseNumber           string
	HouseNumberAddition   string
	Settlement            string
	ZipCode               string
	PostalOffice          string
	Country               string
}

func parsePRSEntry(row []string) (PRSEntry, errors.E) {
	if len(row) != columnsCount {
		errE := errors.New("invalid row: unexpected number of columns")
		errors.Details(errE)["want"] = columnsCount
		errors.Details(errE)["got"] = len(row)
		return PRSEntry{}, errE
	}

	if len(row[0]) != registrationNumberLength {
		errE := errors.New("registration number length error")
		errors.Details(errE)["id"] = row[0]
		return PRSEntry{}, errE
	}

	return PRSEntry{
		RegistrationNumber:    row[0],
		Name:                  row[1],
		HSEID:                 row[2],
		CompanyLegalForm:      row[3],
		RegistrationAuthority: row[4],
		Street:                row[5],
		HouseNumber:           row[6],
		HouseNumberAddition:   row[7],
		Settlement:            row[8],
		ZipCode:               row[9],
		PostalOffice:          row[10],
		Country:               row[11],
	}, nil
}

func processPRSFile(reader io.ReadCloser) ([]PRSEntry, errors.E) {
	defer reader.Close() //nolint:errcheck

	utf16Decoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
	utf8Reader := transform.NewReader(reader, utf16Decoder)

	csvReader := csv.NewReader(utf8Reader)

	var records []PRSEntry
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

		if i == 0 {
			// We skip the header row.
			continue
		}
		entry, errE := parsePRSEntry(row)
		if errE != nil {
			errors.Details(errE)["i"] = i
			return nil, errE
		}

		records = append(records, entry)
	}
	return records, nil
}

func getPRS(ctx context.Context, httpClient *retryablehttp.Client, logger zerolog.Logger, cacheDir, url string) ([]PRSEntry, errors.E) {
	reader, _, errE := indexer.CachedDownload(ctx, httpClient, logger, cacheDir, url)
	if errE != nil {
		return nil, errE
	}
	defer reader.Close() //nolint:errcheck

	records, errE := processPRSFile(reader)
	if errE != nil {
		return nil, errE
	}
	return records, nil
}

func makePRSDoc(prs PRSEntry) (document.D, errors.E) {
	doc := document.D{
		CoreDocument: document.CoreDocument{
			ID:    document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber),
			Score: document.LowConfidence,
		},
		Claims: &document.ClaimTypes{
			Identifier: document.IdentifierClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "COMPANY_REGISTRATION_NUMBER", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("COMPANY_REGISTRATION_NUMBER"),
					Value: prs.RegistrationNumber,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "HSEID", 0),
						Confidence: document.HighConfidence,
					},
					Prop:  document.GetCorePropertyReference("HSEID"),
					Value: prs.HSEID,
				},
			},
			Relation: document.RelationClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "TYPE", 0, "NAME", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("TYPE"),
					To:   document.GetCorePropertyReference("COMPANY"),
				},
			},
			Text: document.TextClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "NAME", 0),
						Confidence: document.HighConfidence,
					},
					Prop: document.GetCorePropertyReference("NAME"),
					HTML: document.TranslatableHTMLString{"en": html.EscapeString(prs.Name)},
				},
			},
			String: document.StringClaims{
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "COMPANY_LEGAL_FORM", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("COMPANY_LEGAL_FORM"),
					String: prs.CompanyLegalForm,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "FINANCIAL_OFFICE", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("FINANCIAL_OFFICE"),
					String: prs.RegistrationAuthority,
				},
				{
					CoreClaim: document.CoreClaim{
						ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "COUNTRY_OF_INCORPORATION", 0),
						Confidence: document.HighConfidence,
					},
					Prop:   document.GetCorePropertyReference("COUNTRY_OF_INCORPORATION"),
					String: "Slovenia",
				},
			},
		},
	}

	if s := strings.TrimSpace(prs.PostalOffice); s != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "POSTAL_OFFICE", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("POSTAL_OFFICE"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(prs.PostalOffice)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	street := strings.TrimSpace(prs.Street)
	if street != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "ADDRESS_STREET", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("ADDRESS_STREET"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(prs.Street)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	houseNumber := strings.TrimSpace(prs.HouseNumber)
	if houseNumber != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "HOUSE_NUMBER", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("HOUSE_NUMBER"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(prs.HouseNumber)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	houseNumberAdd := strings.TrimSpace(prs.HouseNumberAddition)
	if houseNumberAdd != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "HOUSE_NUMBER_ADDITION", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("HOUSE_NUMBER_ADDITION"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(houseNumberAdd)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	zipCode := strings.TrimSpace(prs.ZipCode)
	if zipCode != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "ZIP_CODE", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("ZIP_CODE"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(prs.ZipCode)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	settlement := strings.TrimSpace(prs.Settlement)
	if settlement != "" {
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "SETTLEMENT", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("SETTLEMENT"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(prs.Settlement)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	country := strings.TrimSpace(prs.Country)
	if country != "" {
		errE := doc.Add(&document.StringClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "ADDRESS_COUNTRY", 0),
				Confidence: document.HighConfidence,
			},
			Prop:   document.GetCorePropertyReference("ADDRESS_COUNTRY"),
			String: prs.Country,
		})
		if errE != nil {
			return doc, errE
		}
	}

	if street != "" && houseNumber != "" && zipCode != "" && settlement != "" && country != "" {
		if houseNumberAdd != "" {
			houseNumber += " " + houseNumberAdd
		}
		address := street + " " + houseNumber + ", " + zipCode + " " + settlement + ", " + country
		errE := doc.Add(&document.TextClaim{
			CoreClaim: document.CoreClaim{
				ID:         document.GetID(NameSpaceProducts, "PRS", prs.RegistrationNumber, "ADDRESS", 0),
				Confidence: document.HighConfidence,
			},
			Prop: document.GetCorePropertyReference("ADDRESS"),
			HTML: document.TranslatableHTMLString{"en": html.EscapeString(address)},
		})
		if errE != nil {
			return doc, errE
		}
	}

	return doc, nil
}

//nolint:dupl
func (p PRS) Run(
	ctx context.Context,
	config *Config,
	httpClient *retryablehttp.Client,
	store *store.Store[json.RawMessage, *types.DocumentMetadata, *types.NoMetadata, *types.NoMetadata, *types.NoMetadata, document.Changes],
	indexingCount, indexingSize *x.Counter,
) errors.E {
	if p.Disabled {
		return nil
	}

	records, errE := getPRS(ctx, httpClient, config.Logger, config.CacheDir, prsURL)
	if errE != nil {
		return errE
	}

	config.Logger.Info().Int("total", len(records)).Msg("retrieved PRS data")

	description := "PRS processing"
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
		if err != nil {
			return errors.WithStack(err)
		}
		config.Logger.Debug().
			Int("index", i).
			Str("id", record.RegistrationNumber).
			Msg("processing PRS record")

		doc, errE := makePRSDoc(record)
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
