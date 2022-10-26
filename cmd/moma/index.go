package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
	"gitlab.com/peerdb/search/internal/es"
)

const (
	progressPrintRate = 30 * time.Second
)

var (
	NameSpaceMoMA = uuid.MustParse("d1a7b133-7d73-4ff1-b4d1-0ac93b91cccd")
)

type Artist struct {
	ConstituentID int    `json:"ConstituentID"`
	DisplayName   string `json:"DisplayName"`
	ArtistBio     string `json:"ArtistBio"`
	Nationality   string `json:"Nationality"`
	Gender        string `json:"Gender"`
	BeginDate     int    `json:"BeginDate"`
	EndDate       int    `json:"EndDate"`
	WikiQID       string `json:"Wiki QID"`
	ULAN          string `json:"ULAN"`
}

type Artwork struct {
	Title           string   `json:"Title"`
	Artist          []string `json:"Artist"`
	ConstituentID   []int    `json:"ConstituentID"`
	ArtistBio       []string `json:"ArtistBio"`
	Nationality     []string `json:"Nationality"`
	BeginDate       []int    `json:"BeginDate"`
	EndDate         []int    `json:"EndDate"`
	Gender          []string `json:"Gender"`
	Date            string   `json:"Date"`
	Medium          string   `json:"Medium"`
	Dimensions      string   `json:"Dimensions"`
	CreditLine      string   `json:"CreditLine"`
	AccessionNumber string   `json:"AccessionNumber"`
	Classification  string   `json:"Classification"`
	Department      string   `json:"Department"`
	DateAcquired    string   `json:"DateAcquired"`
	Cataloged       string   `json:"Cataloged"`
	ObjectID        int      `json:"ObjectID"`
	URL             string   `json:"URL"`
	ThumbnailURL    string   `json:"ThumbnailURL"`
	Depth           float64  `json:"Depth (cm),omitempty"`
	Height          float64  `json:"Height (cm),omitempty"`
	Width           float64  `json:"Width (cm),omitempty"`
	Weight          float64  `json:"Weight (kg),omitempty"`
	Diameter        float64  `json:"Diameter (cm),omitempty"`
	Length          float64  `json:"Length (cm),omitempty"`
	Circumference   float64  `json:"Circumference (cm),omitempty"`
	Duration        float64  `json:"Duration (sec.),omitempty"`
}

func getPathAndURL(cacheDir, url string) (string, string) {
	_, err := os.Stat(url)
	if os.IsNotExist(err) {
		return filepath.Join(cacheDir, path.Base(url)), url
	}
	return url, ""
}

func structName(name string) string {
	i := strings.LastIndex(name, ".")
	return strings.ToLower(name[i+1:])
}

func getJSON[T any](ctx context.Context, httpClient *retryablehttp.Client, logger zerolog.Logger, cacheDir, url string) ([]T, errors.E) {
	cachedPath, url := getPathAndURL(cacheDir, url)

	var cachedReader io.Reader
	var cachedSize int64

	cachedFile, err := os.Open(cachedPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, errors.WithStack(err)
		}
		// File does not exists. Continue.
	} else {
		defer cachedFile.Close()
		cachedReader = cachedFile
		cachedSize, err = cachedFile.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		_, err = cachedFile.Seek(0, io.SeekStart)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	if cachedReader == nil {
		// File does not already exist. We download the file and optionally save it.
		req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		downloadReader, errE := x.NewRetryableResponse(httpClient, req)
		if errE != nil {
			return nil, errors.WithStack(err)
		}
		defer downloadReader.Close()
		cachedSize = downloadReader.Size()
		cachedFile, err := os.Create(cachedPath)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		defer func() {
			info, err := os.Stat(cachedPath)
			if err != nil || downloadReader.Size() != info.Size() {
				// Incomplete file. Delete.
				_ = os.Remove(cachedPath)
			}
		}()
		defer cachedFile.Close()
		cachedReader = io.TeeReader(downloadReader, cachedFile)
	}

	progress := es.Progress(logger, nil, nil, nil, fmt.Sprintf("%s download progress", structName(fmt.Sprintf("%T", *new(T)))))
	countingReader := &x.CountingReader{Reader: cachedReader}
	ticker := x.NewTicker(ctx, countingReader, cachedSize, progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	var result []T
	decoder := json.NewDecoder(countingReader)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&result)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return result, nil
}

func getArtistReference(artistsMap map[int]search.Document, constituentID int) (search.DocumentReference, errors.E) {
	doc, ok := artistsMap[constituentID]
	if !ok {
		return search.DocumentReference{}, errors.Errorf(`unknown artist "%d"`, constituentID)
	}
	return doc.Reference(), nil
}

func index(config *Config) errors.E {
	ctx, _, httpClient, esClient, processor, errE := es.Initialize(config.Log, config.Elastic, config.Index)
	if errE != nil {
		return errE
	}

	artists, err := getJSON[Artist](ctx, httpClient, config.Log, config.CacheDir, config.ArtistsURL)
	if err != nil {
		return err
	}

	artworks, err := getJSON[Artwork](ctx, httpClient, config.Log, config.CacheDir, config.ArtworksURL)
	if err != nil {
		return err
	}

	count := x.Counter(0)
	progress := es.Progress(config.Log, processor, nil, nil, "indexing")
	ticker := x.NewTicker(ctx, &count, int64(len(search.CoreProperties))+int64(len(artists))+int64(len(artworks)), progressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	err = search.SaveCoreProperties(ctx, config.Log, esClient, processor, config.Index)
	if err != nil {
		return err
	}

	artistsMap := map[int]search.Document{}

	for _, artist := range artists {
		doc := search.Document{
			CoreDocument: search.CoreDocument{
				ID: search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID),
				Name: search.Name{
					"en": artist.DisplayName,
				},
				Score: 0.0,
			},
			Active: &search.ClaimTypes{
				Identifier: search.IdentifierClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "MOMA_CONSTITUENT_ID", 0),
							Confidence: es.HighConfidence,
						},
						Prop:       search.GetCorePropertyReference("MOMA_CONSTITUENT_ID"),
						Identifier: strconv.Itoa(artist.ConstituentID),
					},
				},
				Reference: search.ReferenceClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "MOMA_CONSTITUENT_PAGE", 0),
							Confidence: es.HighConfidence,
						},
						Prop: search.GetCorePropertyReference("MOMA_CONSTITUENT_PAGE"),
						IRI:  fmt.Sprintf("https://www.moma.org/artists/%d", artist.ConstituentID),
					},
				},
				Relation: search.RelationClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "IS", 0, "ARTIST", 0),
							Confidence: es.HighConfidence,
						},
						Prop: search.GetCorePropertyReference("IS"),
						To:   search.GetCorePropertyReference("ARTIST"),
					},
				},
			},
		}

		if artist.ArtistBio != "" {
			err := doc.Add(&search.TextClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "DESCRIPTION", 0),
					Confidence: es.MediumConfidence,
				},
				Prop: search.GetCorePropertyReference("DESCRIPTION"),
				HTML: search.TranslatableHTMLString{"en": html.EscapeString(artist.ArtistBio)},
			})
			if err != nil {
				return err
			}
		}
		if artist.Nationality != "" {
			err := doc.Add(&search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "NATIONALITY", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("NATIONALITY"),
				String: artist.Nationality,
			})
			if err != nil {
				return err
			}
		}
		if artist.Gender != "" {
			err := doc.Add(&search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "GENDER", 0),
					Confidence: es.HighConfidence,
				},
				Prop: search.GetCorePropertyReference("GENDER"),
				// We convert to lower case because input data does not have uniform case.
				String: strings.ToLower(artist.Gender),
			})
			if err != nil {
				return err
			}
		}
		if artist.BeginDate != 0 {
			err := doc.Add(&search.TimeClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "BEGIN_DATE", 0),
					Confidence: es.HighConfidence,
				},
				Prop:      search.GetCorePropertyReference("BEGIN_DATE"),
				Timestamp: search.Timestamp(time.Date(artist.BeginDate, time.January, 1, 0, 0, 0, 0, time.UTC)),
				Precision: search.TimePrecisionYear,
			})
			if err != nil {
				return err
			}
		}
		if artist.EndDate != 0 {
			err := doc.Add(&search.TimeClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "END_DATE", 0),
					Confidence: es.HighConfidence,
				},
				Prop:      search.GetCorePropertyReference("END_DATE"),
				Timestamp: search.Timestamp(time.Date(artist.EndDate, time.January, 1, 0, 0, 0, 0, time.UTC)),
				Precision: search.TimePrecisionYear,
			})
			if err != nil {
				return err
			}
		}
		if artist.WikiQID != "" {
			err := doc.Add(&search.IdentifierClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "WIKIDATA_ITEM_ID", 0),
					Confidence: es.HighConfidence,
				},
				Prop:       search.GetCorePropertyReference("WIKIDATA_ITEM_ID"),
				Identifier: artist.WikiQID,
			})
			if err != nil {
				return err
			}
			err = doc.Add(&search.ReferenceClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "WIKIDATA_ITEM_PAGE", 0),
					Confidence: es.HighConfidence,
				},
				Prop: search.GetCorePropertyReference("WIKIDATA_ITEM_PAGE"),
				IRI:  fmt.Sprintf("https://www.wikidata.org/wiki/%s", artist.WikiQID),
			})
			if err != nil {
				return err
			}
		}
		if artist.ULAN != "" {
			err := doc.Add(&search.IdentifierClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "ULAN_ID", 0),
					Confidence: es.HighConfidence,
				},
				Prop:       search.GetCorePropertyReference("ULAN_ID"),
				Identifier: artist.ULAN,
			})
			if err != nil {
				return err
			}
			err = doc.Add(&search.ReferenceClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "ULAN_PAGE", 0),
					Confidence: es.HighConfidence,
				},
				Prop: search.GetCorePropertyReference("ULAN_PAGE"),
				IRI:  fmt.Sprintf("https://www.getty.edu/vow/ULANFullDisplay?find=&role=&nation=&subjectid=%s", artist.ULAN),
			})
			if err != nil {
				return err
			}
		}

		artistsMap[artist.ConstituentID] = doc

		count.Increment()

		config.Log.Debug().Str("doc", string(doc.ID)).Msg("saving document")
		search.InsertOrReplaceDocument(processor, config.Index, &doc)
	}

	artworksMap := map[int]search.Document{}

	for _, artwork := range artworks {
		doc := search.Document{
			CoreDocument: search.CoreDocument{
				ID: search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID),
				Name: search.Name{
					"en": artwork.Title,
				},
				Score: 0.0,
			},
			Active: &search.ClaimTypes{
				Identifier: search.IdentifierClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "MOMA_OBJECT_ID", 0),
							Confidence: es.HighConfidence,
						},
						Prop:       search.GetCorePropertyReference("MOMA_OBJECT_ID"),
						Identifier: strconv.Itoa(artwork.ObjectID),
					},
				},
				Reference: search.ReferenceClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "MOMA_OBJECT_PAGE", 0),
							Confidence: es.HighConfidence,
						},
						Prop: search.GetCorePropertyReference("MOMA_OBJECT_PAGE"),
						IRI:  fmt.Sprintf("https://www.moma.org/collection/works/%d", artwork.ObjectID),
					},
				},
				Relation: search.RelationClaims{
					{
						CoreClaim: search.CoreClaim{
							ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "IS", 0, "ARTWORK", 0),
							Confidence: es.HighConfidence,
						},
						Prop: search.GetCorePropertyReference("IS"),
						To:   search.GetCorePropertyReference("ARTWORK"),
					},
				},
			},
		}

		processedConstituentIDs := map[int]bool{}
		for _, constituentID := range artwork.ConstituentID {
			// Skip duplicate artists.
			// See: https://github.com/MuseumofModernArt/collection/issues/25
			if processedConstituentIDs[constituentID] {
				continue
			}
			processedConstituentIDs[constituentID] = true
			to, err := getArtistReference(artistsMap, constituentID)
			if err != nil {
				return err
			}
			err = doc.Add(&search.RelationClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "BY_ARTIST", 0, constituentID),
					Confidence: es.HighConfidence,
				},
				Prop: search.GetCorePropertyReference("BY_ARTIST"),
				To:   to,
			})
			if err != nil {
				return err
			}
		}

		if artwork.Date != "" {
			err := doc.Add(&search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DATE", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("DATE"),
				String: artwork.Date,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Medium != "" {
			err := doc.Add(&search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "MEDIUM", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("MEDIUM"),
				String: artwork.Medium,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Dimensions != "" {
			err := doc.Add(&search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DIMENSIONS", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("DIMENSIONS"),
				String: artwork.Dimensions,
			})
			if err != nil {
				return err
			}
		}
		if artwork.CreditLine != "" {
			err := doc.Add(&search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "CREDIT", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("CREDIT"),
				String: artwork.CreditLine,
			})
			if err != nil {
				return err
			}
		}
		if artwork.AccessionNumber != "" {
			err := doc.Add(&search.IdentifierClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "MOMA_ACCESSION_NUMBER", 0),
					Confidence: es.HighConfidence,
				},
				Prop:       search.GetCorePropertyReference("MOMA_ACCESSION_NUMBER"),
				Identifier: artwork.AccessionNumber,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Classification != "" {
			err := doc.Add(&search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "CLASSIFICATION", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("CLASSIFICATION"),
				String: artwork.Classification,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Department != "" {
			err := doc.Add(&search.StringClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DEPARTMENT", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("DEPARTMENT"),
				String: artwork.Department,
			})
			if err != nil {
				return err
			}
		}
		if artwork.DateAcquired != "" {
			timestamp, err := time.Parse("2006-01-02", artwork.DateAcquired)
			if err != nil {
				return errors.WithStack(err)
			}
			errE := doc.Add(&search.TimeClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DATE_ACQUIRED", 0),
					Confidence: es.HighConfidence,
				},
				Prop:      search.GetCorePropertyReference("DATE_ACQUIRED"),
				Timestamp: search.Timestamp(timestamp),
				Precision: search.TimePrecisionDay,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Cataloged != "" {
			var confidence search.Confidence
			switch artwork.Cataloged {
			case "Y":
				confidence = es.HighConfidence
			case "N":
				confidence = es.HighNegationConfidence
			default:
				return errors.Errorf(`unsupported cataloged value "%s"`, artwork.Cataloged)
			}
			err := doc.Add(&search.RelationClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "LABEL", 0, "CATALOGED", 0),
					Confidence: confidence,
				},
				Prop: search.GetCorePropertyReference("LABEL"),
				To:   search.GetCorePropertyReference("CATALOGED"),
			})
			if err != nil {
				return err
			}
		}
		if artwork.Depth != 0 {
			err := doc.Add(&search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DEPTH", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("DEPTH"),
				Unit:   search.AmountUnitMetre,
				Amount: artwork.Depth * 0.01,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Height != 0 {
			err := doc.Add(&search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "HEIGHT", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("HEIGHT"),
				Unit:   search.AmountUnitMetre,
				Amount: artwork.Height * 0.01,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Width != 0 {
			err := doc.Add(&search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "WIDTH", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("WIDTH"),
				Unit:   search.AmountUnitMetre,
				Amount: artwork.Width * 0.01,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Weight != 0 {
			err := doc.Add(&search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "WEIGHT", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("WEIGHT"),
				Unit:   search.AmountUnitKilogram,
				Amount: artwork.Weight,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Diameter != 0 {
			err := doc.Add(&search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DIAMETER", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("DIAMETER"),
				Unit:   search.AmountUnitMetre,
				Amount: artwork.Diameter * 0.01,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Length != 0 {
			err := doc.Add(&search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "LENGTH", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("LENGTH"),
				Unit:   search.AmountUnitMetre,
				Amount: artwork.Length * 0.01,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Circumference != 0 {
			err := doc.Add(&search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "CIRCUMFERENCE", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("CIRCUMFERENCE"),
				Unit:   search.AmountUnitMetre,
				Amount: artwork.Circumference * 0.01,
			})
			if err != nil {
				return err
			}
		}
		if artwork.Duration != 0 {
			err := doc.Add(&search.AmountClaim{
				CoreClaim: search.CoreClaim{
					ID:         search.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DURATION", 0),
					Confidence: es.HighConfidence,
				},
				Prop:   search.GetCorePropertyReference("DURATION"),
				Unit:   search.AmountUnitSecond,
				Amount: artwork.Duration,
			})
			if err != nil {
				return err
			}
		}

		artworksMap[artwork.ObjectID] = doc

		count.Increment()

		config.Log.Debug().Str("doc", string(doc.ID)).Msg("saving document")
		search.InsertOrReplaceDocument(processor, config.Index, &doc)
	}

	return nil
}
