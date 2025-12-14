package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/peerdb"
	"gitlab.com/peerdb/peerdb/document"
	"gitlab.com/peerdb/peerdb/internal/es"
	"gitlab.com/peerdb/peerdb/internal/indexer"
)

const (
	centimetreToMetre = 0.01
)

var (
	//nolint:gochecknoglobals
	NameSpaceMoMA = uuid.MustParse("d1a7b133-7d73-4ff1-b4d1-0ac93b91cccd")

	mediaRegex     = regexp.MustCompile(`^(?:/media|/d/assets)/([^./]+)(?:/.+)?.(jpg|png)(?:\?sha=\w+)?$`)
	resizeRegex    = regexp.MustCompile(`-resize (\d+)x(\d+)`)
	srcSetSepRegex = regexp.MustCompile(`,\s+`)
)

type picture struct {
	Sources     []string `json:"sources,omitempty"     pagser:"source->eachAttr(srcset)"`
	ImageSrc    string   `json:"imageSrc,omitempty"    pagser:"img->attr(src)"`
	ImageSrcSet string   `json:"imageSrcSet,omitempty" pagser:"img->attr(srcset)"`
}

type imageSrc struct {
	MediaType string
	Path      string
	Width     int
	Height    int
}

type image struct {
	URL       string
	Preview   string
	MediaType string
}

func parseMediaURL(path string) (string, int, int, errors.E) {
	match := mediaRegex.FindStringSubmatch(path)
	if match == nil {
		return "", 0, 0, errors.Errorf(`unsupported path "%s"`, path)
	}
	matchData, err := base64.RawURLEncoding.DecodeString(match[1])
	if err != nil {
		return "", 0, 0, errors.WithStack(err)
	}
	var decodedData [][]string
	errE := x.Unmarshal(matchData, &decodedData)
	if errE != nil {
		return "", 0, 0, errE
	}

	var mediaType string
	switch match[2] {
	case "jpg":
		mediaType = "image/jpeg"
	case "png":
		mediaType = "image/png"
	default:
		return "", 0, 0, errors.Errorf(`unsupported file extension "%s"`, match[2])
	}

	match = resizeRegex.FindStringSubmatch(decodedData[1][2])
	if match == nil {
		return "", 0, 0, errors.Errorf(`unsupported resize argument "%s"`, decodedData[1][2])
	}
	width, err := strconv.Atoi(match[1])
	if err != nil {
		return "", 0, 0, errors.WithStack(err)
	}
	height, err := strconv.Atoi(match[2])
	if err != nil {
		return "", 0, 0, errors.WithStack(err)
	}

	return mediaType, width, height, nil
}

func parseSrcSet(srcSet string) []string {
	result := []string{}
	sets := srcSetSepRegex.Split(srcSet, -1)
	for _, set := range sets {
		if set == "" {
			continue
		}
		i := strings.LastIndex(set, " ")
		if i == -1 {
			result = append(result, set)
		} else {
			result = append(result, set[:i])
		}
	}
	return result
}

func (p picture) Image() (image, errors.E) {
	images := []imageSrc{}

	if p.ImageSrc != "" {
		mediaType, width, height, errE := parseMediaURL(p.ImageSrc)
		if errE != nil {
			return image{}, errE
		}
		images = append(images, imageSrc{
			Path:      p.ImageSrc,
			MediaType: mediaType,
			Width:     width,
			Height:    height,
		})
	}
	for _, path := range parseSrcSet(p.ImageSrcSet) {
		mediaType, width, height, errE := parseMediaURL(path)
		if errE != nil {
			return image{}, errE
		}
		images = append(images, imageSrc{
			Path:      path,
			MediaType: mediaType,
			Width:     width,
			Height:    height,
		})
	}
	for _, source := range p.Sources {
		for _, path := range parseSrcSet(source) {
			mediaType, width, height, errE := parseMediaURL(path)
			if errE != nil {
				return image{}, errE
			}
			images = append(images, imageSrc{
				Path:      path,
				MediaType: mediaType,
				Width:     width,
				Height:    height,
			})
		}
	}

	if len(images) == 0 {
		return image{}, errors.New("no images")
	}

	// Sorts so that the image with the largest area is the first.
	slices.SortStableFunc(images, func(a imageSrc, b imageSrc) int {
		return b.Width*b.Height - a.Width*a.Height
	})
	// There should be always at least one image at this point.
	url := "https://www.moma.org" + images[0].Path
	mediaType := images[0].MediaType

	// Sorts so that the image with the smallest width is the first.
	slices.SortStableFunc(images, func(a imageSrc, b imageSrc) int {
		return a.Width - b.Width
	})
	for len(images) > 0 {
		// Remove all images which are too small for preview by width.
		if images[0].Width < es.PreviewSize {
			images = images[1:]
		} else {
			break
		}
	}

	// Sorts so that the image with the smallest height is the first.
	slices.SortStableFunc(images, func(a imageSrc, b imageSrc) int {
		return a.Height - b.Height
	})
	for len(images) > 0 {
		// Remove all images which are too small for preview by height.
		if images[0].Height < es.PreviewSize {
			images = images[1:]
		} else {
			break
		}
	}

	if len(images) == 0 {
		return image{}, errors.New("no image suitable for preview")
	}

	return image{
		URL:       url,
		Preview:   "https://www.moma.org" + images[0].Path,
		MediaType: mediaType,
	}, nil
}

type momaArtist struct {
	ChallengeRunning bool      `json:"challengeRunning"   pagser:"#challenge-running->exists()"`
	Pictures         []picture `json:"pictures,omitempty" pagser:"#main > div > section[role='banner'] picture"`
	Article          string    `json:"article,omitempty"  pagser:"#main > div > section.\\$typography\\/baseline\\:body section.typography\\/markdown->html()"`
}

type momaArtwork struct {
	ChallengeRunning bool      `json:"challengeRunning"   pagser:"#challenge-running->exists()"`
	Pictures         []picture `json:"pictures,omitempty" pagser:"section.work *:not(button) > picture"`
	Article          string    `json:"article,omitempty"  pagser:"#text->html()"`
}

//nolint:tagliatelle
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

//nolint:tagliatelle
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
	ImageURL        string   `json:"ImageURL"`
	OnView          string   `json:"OnView"`

	Depth         float64 `json:"Depth (cm),omitempty"`
	Height        float64 `json:"Height (cm),omitempty"`
	Width         float64 `json:"Width (cm),omitempty"`
	Weight        float64 `json:"Weight (kg),omitempty"`
	Diameter      float64 `json:"Diameter (cm),omitempty"`
	Length        float64 `json:"Length (cm),omitempty"`
	Circumference float64 `json:"Circumference (cm),omitempty"`
	Duration      float64 `json:"Duration (sec.),omitempty"`
}

func getJSON[T any](ctx context.Context, httpClient *retryablehttp.Client, logger zerolog.Logger, cacheDir, url string) ([]T, errors.E) {
	reader, _, errE := indexer.CachedDownload(ctx, httpClient, logger, cacheDir, url)
	if errE != nil {
		return nil, errE
	}
	defer reader.Close()

	var result []T
	// TODO: We should stream results as they are downloaded/decompressed/decoded like go-mediawiki package does.
	errE = x.DecodeJSONWithoutUnknownFields(reader, &result)
	if errE != nil {
		return nil, errE
	}
	return result, nil
}

func getArtistReference(artistsMap map[int]document.D, constituentID int) (document.Reference, errors.E) {
	doc, ok := artistsMap[constituentID]
	if !ok {
		errE := errors.New("unknown artist")
		errors.Details(errE)["constituentID"] = constituentID
		return document.Reference{}, errE
	}
	return doc.Reference(), nil
}

func getArtist(ctx context.Context, httpClient *retryablehttp.Client, constituentID int) (momaArtist, errors.E) {
	url := fmt.Sprintf("https://www.moma.org/artists/%d", constituentID)
	return indexer.GetWebData[momaArtist](ctx, httpClient, url, indexer.ExtractData)
}

func getArtwork(ctx context.Context, httpClient *retryablehttp.Client, objectID int) (momaArtwork, errors.E) {
	url := fmt.Sprintf("https://www.moma.org/collection/works/%d", objectID)
	return indexer.GetWebData[momaArtwork](ctx, httpClient, url, indexer.ExtractData)
}

func index(config *Config) errors.E { //nolint:maintidx
	ctx, stop, httpClient, store, esClient, esProcessor, errE := es.Standalone(
		config.Logger, string(config.Postgres.URL), config.Elastic.URL, config.Postgres.Schema, config.Elastic.Index,
	)
	if errE != nil {
		return errE
	}
	defer stop()

	artists, errE := getJSON[Artist](ctx, httpClient, config.Logger, config.CacheDir, config.ArtistsURL)
	if errE != nil {
		return errE
	}

	artworks, errE := getJSON[Artwork](ctx, httpClient, config.Logger, config.CacheDir, config.ArtworksURL)
	if errE != nil {
		return errE
	}

	count := x.NewCounter(0)
	size := x.NewCounter(int64(len(artists)) + int64(len(artworks)))
	progress := es.Progress(config.Logger, esProcessor, nil, nil, "indexing")
	ticker := x.NewTicker(ctx, count, size, indexer.ProgressPrintRate)
	defer ticker.Stop()
	go func() {
		for p := range ticker.C {
			progress(ctx, p)
		}
	}()

	errE = peerdb.SaveCoreProperties(ctx, config.Logger, store, esClient, esProcessor, config.Elastic.Index, count, size)
	if errE != nil {
		return errE
	}

	artistsMap := map[int]document.D{}

	for _, artist := range artists {
		if ctx.Err() != nil {
			break
		}

		doc := document.D{ //nolint:dupl
			CoreDocument: document.CoreDocument{
				ID:    document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID),
				Score: document.LowConfidence,
			},
			Claims: &document.ClaimTypes{
				Text: document.TextClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "NAME", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("NAME"),
						HTML: document.TranslatableHTMLString{
							"en": html.EscapeString(artist.DisplayName),
						},
					},
				},
				Identifier: document.IdentifierClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "MOMA_CONSTITUENT_ID", 0),
							Confidence: document.HighConfidence,
						},
						Prop:  document.GetCorePropertyReference("MOMA_CONSTITUENT_ID"),
						Value: strconv.Itoa(artist.ConstituentID),
					},
				},
				Reference: document.ReferenceClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "MOMA_CONSTITUENT_PAGE", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("MOMA_CONSTITUENT_PAGE"),
						IRI:  fmt.Sprintf("https://www.moma.org/artists/%d", artist.ConstituentID),
					},
				},
				Relation: document.RelationClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "TYPE", 0, "ARTIST", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("TYPE"),
						To:   document.GetCorePropertyReference("ARTIST"),
					},
				},
			},
		}

		if artist.ArtistBio != "" {
			errE = doc.Add(&document.TextClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "DESCRIPTION", 0),
					Confidence: document.HighConfidence,
				},
				Prop: document.GetCorePropertyReference("DESCRIPTION"),
				HTML: document.TranslatableHTMLString{"en": html.EscapeString(artist.ArtistBio)},
			})
			if errE != nil {
				return errE
			}
		}
		if artist.Nationality != "" {
			errE = doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "NATIONALITY", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("NATIONALITY"),
				String: artist.Nationality,
			})
			if errE != nil {
				return errE
			}
		}
		if artist.Gender != "" {
			errE = doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "GENDER", 0),
					Confidence: document.HighConfidence,
				},
				Prop: document.GetCorePropertyReference("GENDER"),
				// We convert to lower case because input data does not have uniform case.
				String: strings.ToLower(artist.Gender),
			})
			if errE != nil {
				return errE
			}
		}
		if artist.BeginDate != 0 {
			errE = doc.Add(&document.TimeClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "DATE_OF_BIRTH", 0),
					Confidence: document.HighConfidence,
				},
				Prop:      document.GetCorePropertyReference("DATE_OF_BIRTH"),
				Timestamp: document.Timestamp(time.Date(artist.BeginDate, time.January, 1, 0, 0, 0, 0, time.UTC)),
				Precision: document.TimePrecisionYear,
			})
			if errE != nil {
				return errE
			}
		}
		if artist.EndDate != 0 {
			errE = doc.Add(&document.TimeClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "DATE_OF_DEATH", 0),
					Confidence: document.HighConfidence,
				},
				Prop:      document.GetCorePropertyReference("DATE_OF_DEATH"),
				Timestamp: document.Timestamp(time.Date(artist.EndDate, time.January, 1, 0, 0, 0, 0, time.UTC)),
				Precision: document.TimePrecisionYear,
			})
			if errE != nil {
				return errE
			}
		}
		if artist.WikiQID != "" {
			errE = doc.Add(&document.IdentifierClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "WIKIDATA_ITEM_ID", 0),
					Confidence: document.HighConfidence,
				},
				Prop:  document.GetCorePropertyReference("WIKIDATA_ITEM_ID"),
				Value: artist.WikiQID,
			})
			if errE != nil {
				return errE
			}
			errE = doc.Add(&document.ReferenceClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "WIKIDATA_ITEM_PAGE", 0),
					Confidence: document.HighConfidence,
				},
				Prop: document.GetCorePropertyReference("WIKIDATA_ITEM_PAGE"),
				IRI:  "https://www.wikidata.org/wiki/" + artist.WikiQID,
			})
			if errE != nil {
				return errE
			}
		}
		if artist.ULAN != "" {
			errE = doc.Add(&document.IdentifierClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "ULAN_ID", 0),
					Confidence: document.HighConfidence,
				},
				Prop:  document.GetCorePropertyReference("ULAN_ID"),
				Value: artist.ULAN,
			})
			if errE != nil {
				return errE
			}
			errE = doc.Add(&document.ReferenceClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "ULAN_PAGE", 0),
					Confidence: document.HighConfidence,
				},
				Prop: document.GetCorePropertyReference("ULAN_PAGE"),
				IRI:  "https://www.getty.edu/vow/ULANFullDisplay?find=&role=&nation=&subjectid=" + artist.ULAN,
			})
			if errE != nil {
				return errE
			}
		}

		if config.WebsiteData { //nolint:dupl,nestif
			data, errE := getArtist(ctx, httpClient, artist.ConstituentID) //nolint:govet
			if errE != nil {
				if errors.AllDetails(errE)["code"] == http.StatusNotFound {
					config.Logger.Warn().Str("doc", doc.ID.String()).Int("constituentID", artist.ConstituentID).Msg("artist not found, skipping")
					count.Increment()
					continue
				}
				config.Logger.Warn().Err(errE).Str("doc", doc.ID.String()).Int("constituentID", artist.ConstituentID).Msg("error getting artist data")
			} else if data.ChallengeRunning {
				config.Logger.Warn().Str("doc", doc.ID.String()).Int("constituentID", artist.ConstituentID).Msg("CloudFlare bot blocking")
			} else {
				for i, picture := range data.Pictures {
					image, errE := picture.Image() //nolint:govet
					if errE != nil {
						config.Logger.Warn().Err(errE).Str("doc", doc.ID.String()).Int("constituentID", artist.ConstituentID).Send()
					} else {
						errE = doc.Add(&document.FileClaim{
							CoreClaim: document.CoreClaim{
								ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "IMAGE", i),
								Confidence: document.HighConfidence,
							},
							Prop:      document.GetCorePropertyReference("IMAGE"),
							MediaType: image.MediaType,
							URL:       image.URL,
							Preview:   []string{image.Preview},
						})
						if errE != nil {
							return errE
						}
					}
				}
				// TODO: Cleanup HTML.
				if data.Article != "" {
					errE = doc.Add(&document.TextClaim{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "ARTICLE", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("ARTICLE"),
						HTML: document.TranslatableHTMLString{"en": data.Article},
					})
					if errE != nil {
						return errE
					}
					errE = doc.Add(&document.RelationClaim{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTIST", artist.ConstituentID, "LABEL", 0, "HAS_ARTICLE", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("LABEL"),
						To:   document.GetCorePropertyReference("HAS_ARTICLE"),
					})
					if errE != nil {
						return errE
					}
				}
			}
		}

		artistsMap[artist.ConstituentID] = doc

		count.Increment()

		config.Logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE = peerdb.InsertOrReplaceDocument(ctx, store, &doc)
		if errE != nil {
			return errE
		}
	}

	artworksMap := map[int]document.D{}

	for _, artwork := range artworks {
		if ctx.Err() != nil {
			break
		}

		doc := document.D{ //nolint:dupl
			CoreDocument: document.CoreDocument{
				ID:    document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID),
				Score: document.LowConfidence,
			},
			Claims: &document.ClaimTypes{
				Text: document.TextClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "NAME", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("NAME"),
						HTML: document.TranslatableHTMLString{
							"en": html.EscapeString(artwork.Title),
						},
					},
				},
				Identifier: document.IdentifierClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "MOMA_OBJECT_ID", 0),
							Confidence: document.HighConfidence,
						},
						Prop:  document.GetCorePropertyReference("MOMA_OBJECT_ID"),
						Value: strconv.Itoa(artwork.ObjectID),
					},
				},
				Reference: document.ReferenceClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "MOMA_OBJECT_PAGE", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("MOMA_OBJECT_PAGE"),
						IRI:  fmt.Sprintf("https://www.moma.org/collection/works/%d", artwork.ObjectID),
					},
				},
				Relation: document.RelationClaims{
					{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "TYPE", 0, "ARTWORK", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("TYPE"),
						To:   document.GetCorePropertyReference("ARTWORK"),
					},
				},
			},
		}

		// We first check website data because for skipped artists (those artists which exist in the dataset
		// but not on the website) also artworks are generally not on the website, too.
		if config.WebsiteData { //nolint:dupl,nestif
			data, errE := getArtwork(ctx, httpClient, artwork.ObjectID) //nolint:govet
			if errE != nil {
				if errors.AllDetails(errE)["code"] == http.StatusNotFound {
					config.Logger.Warn().Str("doc", doc.ID.String()).Int("objectID", artwork.ObjectID).Msg("artwork not found, skipping")
					count.Increment()
					continue
				}
				config.Logger.Warn().Err(errE).Str("doc", doc.ID.String()).Int("objectID", artwork.ObjectID).Msg("error getting artwork data")
			} else if data.ChallengeRunning {
				config.Logger.Warn().Str("doc", doc.ID.String()).Int("objectID", artwork.ObjectID).Msg("CloudFlare bot blocking")
			} else {
				for i, picture := range data.Pictures {
					image, errE := picture.Image() //nolint:govet
					if errE != nil {
						config.Logger.Warn().Err(errE).Str("doc", doc.ID.String()).Int("objectID", artwork.ObjectID).Send()
					} else {
						errE = doc.Add(&document.FileClaim{
							CoreClaim: document.CoreClaim{
								ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "IMAGE", i),
								Confidence: document.HighConfidence,
							},
							Prop:      document.GetCorePropertyReference("IMAGE"),
							MediaType: image.MediaType,
							URL:       image.URL,
							Preview:   []string{image.Preview},
						})
						if errE != nil {
							return errE
						}
					}
				}
				// TODO: Cleanup HTML.
				if data.Article != "" {
					errE = doc.Add(&document.TextClaim{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "ARTICLE", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("ARTICLE"),
						HTML: document.TranslatableHTMLString{"en": data.Article},
					})
					if errE != nil {
						return errE
					}
					errE = doc.Add(&document.RelationClaim{
						CoreClaim: document.CoreClaim{
							ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "LABEL", 0, "HAS_ARTICLE", 0),
							Confidence: document.HighConfidence,
						},
						Prop: document.GetCorePropertyReference("LABEL"),
						To:   document.GetCorePropertyReference("HAS_ARTICLE"),
					})
					if errE != nil {
						return errE
					}
				}
			}
		} else if artwork.ImageURL != "" {
			url := artwork.ImageURL
			if strings.HasPrefix(url, "http://") {
				url = strings.Replace(url, "http://", "https://", 1)
			}
			errE = doc.Add(&document.FileClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "IMAGE", 0),
					Confidence: document.HighConfidence,
				},
				Prop:      document.GetCorePropertyReference("IMAGE"),
				MediaType: "image/jpeg",
				URL:       url,
				Preview:   []string{url},
			})
			if errE != nil {
				return errE
			}
		}

		processedConstituentIDs := map[int]bool{}
		for _, constituentID := range artwork.ConstituentID {
			// Skip duplicate artists.
			// See: https://github.com/MuseumofModernArt/collection/issues/25
			if processedConstituentIDs[constituentID] {
				continue
			}
			processedConstituentIDs[constituentID] = true
			to, errE := getArtistReference(artistsMap, constituentID) //nolint:govet
			if errE != nil {
				config.Logger.Warn().Err(errE).Str("doc", doc.ID.String()).Int("objectID", artwork.ObjectID).Send()
				continue
			}
			errE = doc.Add(&document.RelationClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "BY_ARTIST", 0, constituentID),
					Confidence: document.HighConfidence,
				},
				Prop: document.GetCorePropertyReference("BY_ARTIST"),
				To:   to,
			})
			if errE != nil {
				return errE
			}
		}

		if artwork.Date != "" {
			errE = doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DATE_CREATED", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("DATE_CREATED"),
				String: artwork.Date,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Medium != "" {
			errE = doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "MEDIUM", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("MEDIUM"),
				String: artwork.Medium,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Dimensions != "" {
			errE = doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DIMENSIONS", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("DIMENSIONS"),
				String: artwork.Dimensions,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.CreditLine != "" {
			errE = doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "CREDIT", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("CREDIT"),
				String: artwork.CreditLine,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.AccessionNumber != "" {
			errE = doc.Add(&document.IdentifierClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "MOMA_ACCESSION_NUMBER", 0),
					Confidence: document.HighConfidence,
				},
				Prop:  document.GetCorePropertyReference("MOMA_ACCESSION_NUMBER"),
				Value: artwork.AccessionNumber,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Classification != "" {
			errE = doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "CLASSIFICATION", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("CLASSIFICATION"),
				String: artwork.Classification,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Department != "" {
			errE = doc.Add(&document.StringClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DEPARTMENT", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("DEPARTMENT"),
				String: artwork.Department,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.DateAcquired != "" {
			timestamp, err := time.Parse("2006-01-02", artwork.DateAcquired)
			if err != nil {
				return errors.WithStack(err)
			}
			errE = doc.Add(&document.TimeClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DATE_ACQUIRED", 0),
					Confidence: document.HighConfidence,
				},
				Prop:      document.GetCorePropertyReference("DATE_ACQUIRED"),
				Timestamp: document.Timestamp(timestamp),
				Precision: document.TimePrecisionDay,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Cataloged != "" {
			var confidence document.Confidence
			switch artwork.Cataloged {
			case "Y":
				confidence = document.HighConfidence
			case "N":
				confidence = document.HighNegationConfidence
			default:
				return errors.Errorf(`unsupported cataloged value "%s"`, artwork.Cataloged)
			}
			errE = doc.Add(&document.RelationClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "LABEL", 0, "CATALOGED", 0),
					Confidence: confidence,
				},
				Prop: document.GetCorePropertyReference("LABEL"),
				To:   document.GetCorePropertyReference("CATALOGED"),
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Depth != 0 {
			errE = doc.Add(&document.AmountClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DEPTH", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("DEPTH"),
				Unit:   document.AmountUnitMetre,
				Amount: artwork.Depth * centimetreToMetre,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Height != 0 {
			errE = doc.Add(&document.AmountClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "HEIGHT", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("HEIGHT"),
				Unit:   document.AmountUnitMetre,
				Amount: artwork.Height * centimetreToMetre,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Width != 0 {
			errE = doc.Add(&document.AmountClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "WIDTH", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("WIDTH"),
				Unit:   document.AmountUnitMetre,
				Amount: artwork.Width * centimetreToMetre,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Weight != 0 {
			errE = doc.Add(&document.AmountClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "WEIGHT", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("WEIGHT"),
				Unit:   document.AmountUnitKilogram,
				Amount: artwork.Weight,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Diameter != 0 {
			errE = doc.Add(&document.AmountClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DIAMETER", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("DIAMETER"),
				Unit:   document.AmountUnitMetre,
				Amount: artwork.Diameter * centimetreToMetre,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Length != 0 {
			errE = doc.Add(&document.AmountClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "LENGTH", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("LENGTH"),
				Unit:   document.AmountUnitMetre,
				Amount: artwork.Length * centimetreToMetre,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Circumference != 0 {
			errE = doc.Add(&document.AmountClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "CIRCUMFERENCE", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("CIRCUMFERENCE"),
				Unit:   document.AmountUnitMetre,
				Amount: artwork.Circumference * centimetreToMetre,
			})
			if errE != nil {
				return errE
			}
		}
		if artwork.Duration != 0 {
			errE = doc.Add(&document.AmountClaim{
				CoreClaim: document.CoreClaim{
					ID:         document.GetID(NameSpaceMoMA, "ARTWORK", artwork.ObjectID, "DURATION", 0),
					Confidence: document.HighConfidence,
				},
				Prop:   document.GetCorePropertyReference("DURATION"),
				Unit:   document.AmountUnitSecond,
				Amount: artwork.Duration,
			})
			if errE != nil {
				return errE
			}
		}

		artworksMap[artwork.ObjectID] = doc

		count.Increment()

		config.Logger.Debug().Str("doc", doc.ID.String()).Msg("saving document")
		errE = peerdb.InsertOrReplaceDocument(ctx, store, &doc)
		if errE != nil {
			return errE
		}
	}

	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	// We wait for everything to be indexed into ElasticSearch.
	// TODO: Improve this to not have a busy wait.
	for {
		err := esProcessor.Flush()
		if err != nil {
			return errors.WithStack(err)
		}
		stats := esProcessor.Stats()
		c := count.Count()
		if c <= stats.Indexed {
			break
		}
		time.Sleep(time.Second)
	}

	_, err := esClient.Refresh(config.Elastic.Index).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	stats := esProcessor.Stats()
	config.Logger.Info().
		Int64("count", count.Count()).
		Int64("total", size.Count()).
		Int64("failed", stats.Failed).Int64("indexed", stats.Succeeded).
		Msg("indexing done")

	return nil
}
