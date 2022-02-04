package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
)

const (
	// TODO: Determine full latest dump dynamically (not in progress/partial).
	latestCommonsImages = "https://dumps.wikimedia.org/commonswiki/20220120/commonswiki-20220120-image.sql.gz"

	highConfidence   = 1.0
	mediumConfidence = 0.5
)

var (
	NameSpaceWikipediaFile = uuid.MustParse("31974ea8-ab0c-466d-9aaa-e1bf3c959edc")

	thumbnailExtraExtensions = map[string]string{
		"image/vnd.djvu":  ".jpg",
		"application/pdf": ".jpg",
		"application/sla": ".png",
		"video/mpeg":      ".jpg",
		"video/ogg":       ".jpg",
		"video/webm":      ".jpg",
		"image/tiff":      ".jpg",
		"image/webp":      ".jpg",
		"image/x-xcf":     ".png",
		"image/svg+xml":   ".png",
	}
	hasPages = map[string]bool{
		"image/vnd.djvu":  true,
		"application/pdf": true,
	}
	noPreview = map[string]bool{
		"audio/webm": true,
		"audio/ogg":  true,
		"audio/midi": true,
		"audio/flac": true,
		"audio/wav":  true,
		"audio/mpeg": true,
	}
)

type Image struct {
	Name          string                 `json:"img_name"`
	Size          int64                  `json:"img_size"`
	Width         int64                  `json:"img_width"`
	Height        int64                  `json:"img_height"`
	Metadata      map[string]interface{} `json:"-"`
	Bits          int64                  `json:"img_bits"`
	MediaType     string                 `json:"img_media_type"`
	MajorMIME     string                 `json:"img_major_mime"`
	MinorMIME     string                 `json:"img_minor_mime"`
	DescriptionID int64                  `json:"img_description_id"`
	ActorID       int64                  `json:"img_actor"`
	Timestamp     time.Time              `json:"-"`
	SHA1          string                 `json:"img_sha1"`
}

func (i *Image) UnmarshalJSON(b []byte) error {
	type ImageSub Image
	type ImageFull struct {
		ImageSub

		Metadata  string `json:"img_metadata"`
		Timestamp string `json:"img_timestamp"`
	}
	var ii ImageFull
	errE := x.UnmarshalWithoutUnknownFields(b, &ii)
	if errE != nil {
		return errE
	}
	metadata, errE := mediawiki.DecodeImageMetadata(ii.Metadata)
	if errE != nil {
		return errE
	}
	timestamp, err := time.ParseInLocation("20060102150405", ii.Timestamp, time.UTC)
	if err != nil {
		return errors.WithStack(errE)
	}

	*i = Image(ii.ImageSub)
	i.Metadata = metadata
	i.Timestamp = timestamp

	return nil
}

type CommonsImagesCommand struct{}

func (c *CommonsImagesCommand) Run(globals *Globals) errors.E {
	ctx := context.Background()

	// We call cancel on SIGINT or SIGTERM signal.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Call cancel on SIGINT or SIGTERM signal.
	go func() {
		c := make(chan os.Signal, 1)
		defer close(c)

		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(c)

		// We wait for a signal or that the context is canceled
		// or that all goroutines are done.
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	client := retryablehttp.NewClient()
	client.RetryWaitMax = clientRetryWaitMax
	client.RetryMax = clientRetryMax

	// We silent debug logging from HTTP client.
	// TODO: Configure proper logger.
	client.Logger = nullLogger{}

	// Set User-Agent header.
	client.RequestLogHook = func(logger retryablehttp.Logger, req *http.Request, retry int) {
		// TODO: Make contact e-mail into a CLI argument.
		req.Header.Set("User-Agent", fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision))
	}

	esClient, errE := search.EnsureIndex(ctx, client.HTTPClient)
	if errE != nil {
		return errE
	}

	// TODO: Make number of workers configurable.
	processor, err := esClient.BulkProcessor().Workers(bulkProcessorWorkers).Stats(true).After(
		func(executionId int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
			if err != nil {
				fmt.Fprintf(os.Stderr, "Indexing error: %s\n", err.Error())
			} else if response.Errors {
				for _, failed := range response.Failed() {
					fmt.Fprintf(os.Stderr, "Indexing error %d (%s): %s [type=%s]\n", failed.Status, http.StatusText(failed.Status), failed.Error.Reason, failed.Error.Type)
				}
				fmt.Fprintf(os.Stderr, "Indexing error\n")
			}
		},
	).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	defer processor.Close()

	return mediawiki.Process(ctx, &mediawiki.ProcessConfig{
		URL:       latestCommonsImages,
		CacheDir:  globals.CacheDir,
		CacheGlob: "commonswiki-*-image.sql.gz",
		CacheFilename: func(_ *http.Response) (string, errors.E) {
			return "commonswiki-20220120-image.sql.gz", nil
		},
		Client:                 client,
		DecompressionThreads:   0,
		DecodingThreads:        0,
		ItemsProcessingThreads: 0,
		Process: func(ctx context.Context, i interface{}) errors.E {
			return c.processImage(ctx, globals, client, processor, *i.(*Image))
		},
		Progress: func(ctx context.Context, p x.Progress) {
			stats := processor.Stats()
			fmt.Fprintf(os.Stderr, "Progress: %0.2f%%, ETA: %s, indexed: %d, failed: %d\n", p.Percent(), p.Remaining().Truncate(time.Second), stats.Succeeded, stats.Failed)
		},
		Item:        &Image{},
		FileType:    mediawiki.SQLDump,
		Compression: mediawiki.GZIP,
	})
}

func getWikimediaCommonsFilePrefix(filename string) string {
	sum := md5.Sum([]byte(filename)) //nolint:gosec
	digest := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s/%s", digest[0:1], digest[0:2])
}

func getPathInt(metadata map[string]interface{}, path []string) int {
	for {
		if len(path) == 0 {
			return 0
		}

		head := path[0]
		tail := path[1:]

		data, ok := metadata[head]
		if !ok {
			return 0
		}
		if len(tail) == 0 {
			dataString, ok := data.(string)
			if !ok {
				return 0
			}
			dataInt, err := strconv.Atoi(dataString)
			if err != nil {
				return 0
			}
			return dataInt
		}
		dataMap, ok := data.(map[string]interface{})
		if !ok {
			return 0
		}

		metadata = dataMap
		path = tail
	}
}

func getPathString(metadata map[string]interface{}, path []string) string {
	for {
		if len(path) == 0 {
			return ""
		}

		head := path[0]
		tail := path[1:]

		data, ok := metadata[head]
		if !ok {
			return ""
		}
		if len(tail) == 0 {
			dataString, ok := data.(string)
			if !ok {
				return ""
			}
			return dataString
		}
		dataMap, ok := data.(map[string]interface{})
		if !ok {
			return ""
		}

		metadata = dataMap
		path = tail
	}
}

func getPathSliceLen(metadata map[string]interface{}, path []string) int {
	for {
		if len(path) == 0 {
			return 0
		}

		head := path[0]
		tail := path[1:]

		data, ok := metadata[head]
		if !ok {
			return 0
		}
		if len(tail) == 0 {
			dataSlice, ok := data.([]interface{})
			if !ok {
				return 0
			}
			return len(dataSlice)
		}
		dataMap, ok := data.(map[string]interface{})
		if !ok {
			return 0
		}

		metadata = dataMap
		path = tail
	}
}

func getPageCount(image Image) int {
	count := getPathInt(image.Metadata, []string{"data", "Pages"})
	if count != 0 {
		return count
	}
	count = getPathInt(image.Metadata, []string{"Pages"})
	if count != 0 {
		return count
	}
	count = getPathSliceLen(image.Metadata, []string{"data", "data", "pages"})
	if count != 0 {
		return count
	}
	blobID := getPathString(image.Metadata, []string{"blobs", "data"})
	if strings.HasPrefix(blobID, "tt:") {
		return 1
	}
	return 0
}

func (c *CommonsImagesCommand) processImage(
	ctx context.Context, globals *Globals, client *retryablehttp.Client, processor *elastic.BulkProcessor, image Image,
) errors.E {
	id := search.GetID(NameSpaceWikipediaFile, image.Name)
	prefix := getWikimediaCommonsFilePrefix(image.Name)
	mediaType := fmt.Sprintf("%s/%s", image.MajorMIME, image.MinorMIME)

	pageCount := 0
	if hasPages[mediaType] {
		pageCount = getPageCount(image)
	}

	previewPages := pageCount
	if previewPages == 0 {
		previewPages = 1
	}
	preview := []string{}
	if !noPreview[mediaType] {
		for page := 1; page <= previewPages; page++ {
			pagePrefix := ""
			if hasPages[mediaType] {
				pagePrefix = fmt.Sprintf("page%d-", page)
			}
			extraExtension := ""
			if thumbnailExtraExtensions[mediaType] != "" {
				extraExtension = thumbnailExtraExtensions[mediaType]
			}
			preview = append(preview,
				fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/thumb/%s/%s/%s256px-%s%s", prefix, image.Name, pagePrefix, image.Name, extraExtension),
			)
		}
	}

	document := search.Document{
		CoreDocument: search.CoreDocument{
			ID: id,
			Name: search.Name{
				"en": strings.ReplaceAll(image.Name, "_", " "),
			},
			Score: 0.0,
		},
		Active: &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikipediaFile, image.Name, "WIKIMEDIA_COMMONS_FILE_NAME", 0),
						Confidence: highConfidence,
					},
					Prop:       search.GetStandardPropertyReference("WIKIMEDIA_COMMONS_FILE_NAME"),
					Identifier: image.Name,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikipediaFile, image.Name, "WIKIMEDIA_COMMONS_FILE", 0),
						Confidence: highConfidence,
					},
					Prop: search.GetStandardPropertyReference("WIKIMEDIA_COMMONS_FILE"),
					IRI:  fmt.Sprintf("https://commons.wikimedia.org/wiki/File:%s", image.Name),
				},
			},
			Relation: search.RelationClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikipediaFile, image.Name, "IS", "FILE", 0),
						Confidence: highConfidence,
					},
					Prop: search.GetStandardPropertyReference("IS"),
					To:   search.GetStandardPropertyReference("FILE"),
				},
			},
			File: search.FileClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikipediaFile, image.Name, "DATA", 0),
						Confidence: highConfidence,
					},
					Prop:    search.GetStandardPropertyReference("DATA"),
					Type:    mediaType,
					URL:     fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/%s/%s", prefix, image.Name),
					Preview: preview,
				},
			},
			String: search.StringClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikipediaFile, image.Name, "MEDIA_TYPE", 0),
						Confidence: highConfidence,
					},
					Prop:   search.GetStandardPropertyReference("MEDIA_TYPE"),
					String: mediaType,
				},
			},
		},
	}

	// TODO: Store other metadata from the image table (width, height, bits, media type (category), size).

	if pageCount > 0 {
		document.Active.Amount = append(document.Active.Amount, search.AmountClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(NameSpaceWikipediaFile, image.Name, "PAGE_COUNT", 0),
				Confidence: mediumConfidence,
			},
			Prop:   search.GetStandardPropertyReference("PAGE_COUNT"),
			Amount: float64(pageCount),
			Unit:   search.AmountUnitNone,
		})
	}

	req := elastic.NewBulkIndexRequest().Index("docs").Id(string(document.ID)).Doc(document)
	processor.Add(req)

	return nil
}
