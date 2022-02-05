package wikipedia

import (
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"

	"gitlab.com/peerdb/search"
)

var (
	NameSpaceWikimediaCommonsFile = uuid.MustParse("31974ea8-ab0c-466d-9aaa-e1bf3c959edc")

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

func GetWikimediaCommonsFileDocumentID(id string) search.Identifier {
	return search.GetID(NameSpaceWikimediaCommonsFile, id)
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
			dataString, ok := data.(string) //nolint:govet
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
			dataSlice, ok := data.([]interface{}) //nolint:govet
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

type xmlParam struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type xmlObject struct {
	Height int        `xml:"height,attr"`
	Width  int        `xml:"width,attr"`
	Params []xmlParam `xml:"PARAM"`
}

type xmlBody struct {
	Objects []xmlObject `xml:"OBJECT"`
}

type xmlData struct {
	Body xmlBody `xml:"BODY"`
}

type xmlDjvu struct {
	Data xmlData `xml:"DjVuXML"`
}

func getXMLPageCount(metadata map[string]interface{}, path []string) int {
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
			var djvu xmlDjvu
			err := xml.Unmarshal([]byte(dataString), &djvu)
			if err != nil {
				return 0
			}
			return len(djvu.Data.Body.Objects)
		}
		dataMap, ok := data.(map[string]interface{})
		if !ok {
			return 0
		}

		metadata = dataMap
		path = tail
	}
}

func getPageCount(ctx context.Context, client *retryablehttp.Client, image Image) int {
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
	count = getXMLPageCount(image.Metadata, []string{"xml"})
	if count != 0 {
		return count
	}
	count = getXMLPageCount(image.Metadata, []string{"data", "xml"})
	if count != 0 {
		return count
	}
	imageInfo, err := getImageInfo(ctx, client, image.Name)
	if err != nil {
		fmt.Fprintf(os.Stderr, `unable to get image info for "%s": %s`+"\n", image.Name, err.Error())
		return 0
	}
	return imageInfo.PageCount
}

func ConvertImage(ctx context.Context, client *retryablehttp.Client, image Image) (*search.Document, errors.E) {
	id := GetWikimediaCommonsFileDocumentID(image.Name)
	prefix := getWikimediaCommonsFilePrefix(image.Name)
	mediaType := fmt.Sprintf("%s/%s", image.MajorMIME, image.MinorMIME)

	pageCount := 0
	if hasPages[mediaType] {
		pageCount = getPageCount(ctx, client, image)
		if pageCount == 0 {
			fmt.Fprintf(os.Stderr, `file "%s" is missing pages metadata`+"\n", image.Name)
		}
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

	name := strings.ReplaceAll(image.Name, "_", " ")
	name = strings.TrimSuffix(name, path.Ext(name))

	document := search.Document{
		CoreDocument: search.CoreDocument{
			ID: id,
			Name: search.Name{
				"en": name,
			},
			Score: 0.0,
		},
		Active: &search.ClaimTypes{
			Identifier: search.IdentifierClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikimediaCommonsFile, image.Name, "WIKIMEDIA_COMMONS_FILE_NAME", 0),
						Confidence: highConfidence,
					},
					Prop:       search.GetStandardPropertyReference("WIKIMEDIA_COMMONS_FILE_NAME"),
					Identifier: image.Name,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikimediaCommonsFile, image.Name, "WIKIMEDIA_COMMONS_FILE", 0),
						Confidence: highConfidence,
					},
					Prop: search.GetStandardPropertyReference("WIKIMEDIA_COMMONS_FILE"),
					IRI:  fmt.Sprintf("https://commons.wikimedia.org/wiki/File:%s", image.Name),
				},
			},
			Relation: search.RelationClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikimediaCommonsFile, image.Name, "IS", "FILE", 0),
						Confidence: highConfidence,
					},
					Prop: search.GetStandardPropertyReference("IS"),
					To:   search.GetStandardPropertyReference("FILE"),
				},
			},
			File: search.FileClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(NameSpaceWikimediaCommonsFile, image.Name, "DATA", 0),
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
						ID:         search.GetID(NameSpaceWikimediaCommonsFile, image.Name, "MEDIA_TYPE", 0),
						Confidence: highConfidence,
					},
					Prop:   search.GetStandardPropertyReference("MEDIA_TYPE"),
					String: mediaType,
				},
			},
		},
	}

	// TODO: Store other metadata from the image table (width, height, bits, media type (category), size).
	// TODO: Store audio/video length.

	if pageCount > 0 {
		document.Active.Amount = append(document.Active.Amount, search.AmountClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(NameSpaceWikimediaCommonsFile, image.Name, "PAGE_COUNT", 0),
				Confidence: mediumConfidence,
			},
			Prop:   search.GetStandardPropertyReference("PAGE_COUNT"),
			Amount: float64(pageCount),
			Unit:   search.AmountUnitNone,
		})
	}

	return &document, nil
}
