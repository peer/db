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

	// We have a list of media types we support primarily to make sure we use consistent media
	// types and to know if we have to map an unknown one to a known one, or to add a new one.
	supportedMediaTypes = map[string]bool{
		"application/pdf": true,
		"application/sla": true,
		"audio/flac":      true,
		"audio/midi":      true,
		"audio/mpeg":      true,
		"audio/ogg":       true,
		"audio/wav":       true,
		"audio/webm":      true,
		"image/gif":       true,
		"image/jpeg":      true,
		"image/png":       true,
		"image/svg+xml":   true,
		"image/tiff":      true,
		"image/vnd.djvu":  true,
		"image/webp":      true,
		"image/x-xcf":     true,
		"video/mpeg":      true,
		"video/ogg":       true,
		"video/webm":      true,
	}
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
		"image/tiff":      true,
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
			switch d := data.(type) {
			case float64:
				return int(d)
			case int64:
				return int(d)
			case string:
				dataInt, err := strconv.Atoi(d)
				if err == nil {
					return dataInt
				}
			case []interface{}:
				return len(d)
			}
			return 0
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
	count = getPathInt(image.Metadata, []string{"data", "page_count"})
	if count != 0 {
		return count
	}
	count = getPathInt(image.Metadata, []string{"page_count"})
	if count != 0 {
		return count
	}
	count = getPathInt(image.Metadata, []string{"data", "data", "pages"})
	if count != 0 {
		return count
	}
	count = getXMLPageCount(image.Metadata, []string{"data", "xml"})
	if count != 0 {
		return count
	}
	count = getXMLPageCount(image.Metadata, []string{"xml"})
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

// Implementation matches includes/media/MediaHandler.php's fitBoxWidth of MediaWiki.
func fitBoxWidth(width, height float64) int {
	idealWidth := width * 256.0 / height
	roundedUp := math.Ceil(idealWidth)
	if math.Round(roundedUp*height/width) > 256.0 {
		return int(math.Floor(idealWidth))
	}
	return int(roundedUp)
}

func ConvertImage(ctx context.Context, client *retryablehttp.Client, image Image) (*search.Document, errors.E) {
	id := GetWikimediaCommonsFileDocumentID(image.Name)
	prefix := getWikimediaCommonsFilePrefix(image.Name)
	mediaType := fmt.Sprintf("%s/%s", image.MajorMIME, image.MinorMIME)
	// Wikimedia Commons uses "application/ogg" for both video and audio, but we find it more informative
	// to tell if it is audio or video through the media type, if this information is already available.
	if mediaType == "application/ogg" {
		if image.MediaType == "AUDIO" {
			mediaType = "audio/ogg"
		} else {
			mediaType = "video/ogg"
		}
	}
	if mediaType == "audio/x-flac" {
		mediaType = "audio/flac"
	}
	if !supportedMediaTypes[mediaType] {
		return nil, errors.Errorf(`unsupported media type "%s" for "%s"`, mediaType, image.Name)
	}

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
		width := 256
		if image.Height > image.Width {
			// Height is at least 1 here, because it is strictly larger than width, which can be at least 0.
			width = fitBoxWidth(float64(image.Width), float64(image.Height))
		}
		for page := 1; page <= previewPages; page++ {
			pagePrefix := ""
			if hasPages[mediaType] {
				pagePrefix = fmt.Sprintf("page%d-", page)
			}
			extraExtension := ""
			if thumbnailExtraExtensions[mediaType] != "" {
				extraExtension = thumbnailExtraExtensions[mediaType]
			}
			extraDash := ""
			// For some reason video/webm files have an extra dash. It works also without,
			// but we want to match thumbnails generated by MediaWiki as close as possible
			// (to use any cache which might exist there).
			if mediaType == "video/webm" {
				extraDash = "-"
			}
			if mediaType == "image/tiff" {
				// For some reason image/tiff files have "lossy" prefix. It works also without,
				// but we want to match thumbnails generated by MediaWiki as close as possible
				// (to use any cache which might exist there). We reuse pagePrefix for this.
				pagePrefix = "lossy-" + pagePrefix
			}
			thumbName := image.Name
			if len(thumbName) > 160 {
				// Too long names are shortened. It works also with the long name, but we want
				// to match thumbnails generated by MediaWiki as close as possible (to use
				// any cache which might exist there).
				ext := path.Ext(thumbName)
				if ext == "" || ext == "." {
					thumbName = "thumbnail"
				} else {
					thumbName = "thumbnail" + ext
				}
			}
			preview = append(preview,
				fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/thumb/%s/%s/%s%dpx-%s%s%s", prefix, image.Name, pagePrefix, width, extraDash, thumbName, extraExtension),
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
