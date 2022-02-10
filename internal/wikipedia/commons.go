package wikipedia

import (
	"context"
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"math"
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

const (
	previewSize  = 256
	longFilename = 160
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
	// See: https://www.mediawiki.org/wiki/Manual:Image_table#img_media_type
	supportedMediawikiMediaTypes = map[string]bool{
		"UNKNOWN":    true,
		"BITMAP":     true,
		"DRAWING":    true,
		"AUDIO":      true,
		"VIDEO":      true,
		"MULTIMEDIA": true,
		"OFFICE":     true,
		"TEXT":       true,
		"EXECUTABLE": true,
		"ARCHIVE":    true,
		"3D":         true,
	}
	thumbnailExtraExtensions = map[string]string{
		"image/vnd.djvu":  ".jpg",
		"application/pdf": ".jpg",
		"application/sla": ".png",
		"video/mpeg":      ".jpg",
		"video/ogg":       ".jpg",
		"video/webm":      ".jpg",
		"image/tiff":      ".jpg",
		"image/webp":      ".png",
		"image/x-xcf":     ".png",
		"image/svg+xml":   ".png",
	}
	hasPages = map[string]bool{
		"image/vnd.djvu":  true,
		"application/pdf": true,
		"image/tiff":      true,
	}
	// TODO: Add audio/midi. See: https://phabricator.wikimedia.org/T301323
	// TODO: Duration for image/webp is not really provided. See: https://phabricator.wikimedia.org/T301332
	hasDuration = map[string]bool{
		"audio/flac": true,
		"audio/mpeg": true,
		"audio/ogg":  true,
		"audio/wav":  true,
		"audio/webm": true,
		"video/mpeg": true,
		"video/ogg":  true,
		"video/webm": true,
		"image/gif":  true,
		"image/png":  true,
		"image/webp": true,
	}
	canHaveZeroDuration = map[string]bool{
		"image/gif":  true,
		"image/png":  true,
		"image/webp": true,
	}
	noPreview = map[string]bool{
		"audio/webm": true,
		"audio/ogg":  true,
		"audio/midi": true,
		"audio/flac": true,
		"audio/wav":  true,
		"audio/mpeg": true,
	}
	browsersSupport = map[string]bool{
		"image/gif":  true,
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
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

func getMediawikiFilePrefix(filename string) string {
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

func getPathFloat(metadata map[string]interface{}, path []string) *float64 {
	for {
		if len(path) == 0 {
			return nil
		}

		head := path[0]
		tail := path[1:]

		data, ok := metadata[head]
		if !ok {
			return nil
		}
		if len(tail) == 0 {
			switch d := data.(type) {
			case float64:
				return &d
			case int64:
				f := float64(d)
				return &f
			case string:
				dataFloat, err := strconv.ParseFloat(d, 64) //nolint:gomnd
				if err == nil {
					return &dataFloat
				}
			}
			return nil
		}
		dataMap, ok := data.(map[string]interface{})
		if !ok {
			return nil
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
			dataString, ok := data.(string) //nolint:govet
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

func getPageCount(ctx context.Context, httpClient *retryablehttp.Client, image Image) (int, errors.E) {
	count := getPathInt(image.Metadata, []string{"data", "Pages"})
	if count != 0 {
		return count, nil
	}
	count = getPathInt(image.Metadata, []string{"Pages"})
	if count != 0 {
		return count, nil
	}
	count = getPathInt(image.Metadata, []string{"data", "page_count"})
	if count != 0 {
		return count, nil
	}
	count = getPathInt(image.Metadata, []string{"page_count"})
	if count != 0 {
		return count, nil
	}
	count = getPathInt(image.Metadata, []string{"data", "data", "pages"})
	if count != 0 {
		return count, nil
	}
	count = getXMLPageCount(image.Metadata, []string{"data", "xml"})
	if count != 0 {
		return count, nil
	}
	count = getXMLPageCount(image.Metadata, []string{"xml"})
	if count != 0 {
		return count, nil
	}
	imageInfo, err := getImageInfo(ctx, httpClient, image.Name)
	if err != nil {
		return 0, errors.Errorf(`unable to get image info for "%s": %w`, image.Name, err)
	}
	return imageInfo.PageCount, nil
}

func getDuration(ctx context.Context, httpClient *retryablehttp.Client, image Image) (float64, errors.E) {
	duration := getPathFloat(image.Metadata, []string{"data", "duration"})
	if duration != nil {
		return *duration, nil
	}
	duration = getPathFloat(image.Metadata, []string{"duration"})
	if duration != nil {
		return *duration, nil
	}
	duration = getPathFloat(image.Metadata, []string{"data", "playtime_seconds"})
	if duration != nil {
		return *duration, nil
	}
	duration = getPathFloat(image.Metadata, []string{"playtime_seconds"})
	if duration != nil {
		return *duration, nil
	}
	duration = getPathFloat(image.Metadata, []string{"data", "length"})
	if duration != nil {
		return *duration, nil
	}
	duration = getPathFloat(image.Metadata, []string{"length"})
	if duration != nil {
		return *duration, nil
	}
	return 0.0, nil
}

// Implementation matches includes/media/MediaHandler.php's fitBoxWidth of MediaWiki.
func fitBoxWidth(width, height float64) int {
	previewSizeFloat := float64(previewSize)
	idealWidth := width * previewSizeFloat / height
	roundedUp := math.Ceil(idealWidth)
	if math.Round(roundedUp*height/width) > previewSizeFloat {
		return int(math.Floor(idealWidth))
	}
	return int(roundedUp)
}

func ConvertWikimediaCommonsImage(ctx context.Context, httpClient *retryablehttp.Client, image Image) (*search.Document, errors.E) {
	return convertImage(ctx, httpClient, NameSpaceWikimediaCommonsFile, "commons", "commons.wikimedia.org", "WIKIMEDIA_COMMONS", image)
}

func convertImage(ctx context.Context, httpClient *retryablehttp.Client, namespace uuid.UUID, fileSite, fileDomain, mnemonicPrefix string, image Image) (*search.Document, errors.E) {
	id := search.GetID(namespace, image.Name)
	prefix := getMediawikiFilePrefix(image.Name)
	mediaType := fmt.Sprintf("%s/%s", image.MajorMIME, image.MinorMIME)
	// Mediawiki uses "application/ogg" for both video and audio, but we find it more informative
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
		return nil, errors.Errorf(`%w: unsupported media type "%s" for "%s"`, notSupportedError, mediaType, image.Name)
	}
	if !supportedMediawikiMediaTypes[image.MediaType] {
		return nil, errors.Errorf(`%w: unsupported Mediawiki media type "%s" for "%s"`, notSupportedError, image.MediaType, image.Name)
	}
	if image.Size == 0 {
		return nil, errors.Errorf("%w: zero size for \"%s\"", SkippedError, image.Name)
	}

	var err errors.E
	pageCount := 0
	if hasPages[mediaType] {
		pageCount, err = getPageCount(ctx, httpClient, image)
		if err != nil {
			// Error happens if there was a problem using the API. This could mean that the file
			// does not exist anymore. In any case, we skip it.
			return nil, errors.Errorf("%w: error getting page count \"%s\": %s", SkippedError, image.Name, err.Error())
		}
		if pageCount == 0 {
			return nil, errors.Errorf("%w: zero page count for \"%s\"", SkippedError, image.Name)
		}
	}

	duration := 0.0
	if hasDuration[mediaType] {
		duration, err = getDuration(ctx, httpClient, image)
		if err != nil {
			// Error happens if there was a problem using the API. This could mean that the file
			// does not exist anymore. In any case, we skip it.
			return nil, errors.Errorf("%w: error getting duration \"%s\": %s", SkippedError, image.Name, err.Error())
		}
		if duration == 0.0 && !canHaveZeroDuration[mediaType] {
			return nil, errors.Errorf("%w: zero duration for \"%s\"", SkippedError, image.Name)
		}
	}

	previewPages := pageCount
	if previewPages == 0 {
		previewPages = 1
	}
	preview := []string{}
	if !noPreview[mediaType] {
		if image.Width == 0 || image.Height == 0 {
			return nil, errors.Errorf("%w: invalid width/height (%dx%d) for \"%s\"", SkippedError, image.Width, image.Height, image.Name)
		}

		if browsersSupport[mediaType] && !hasPages[mediaType] && image.Width <= int64(previewSize) && image.Height <= int64(previewSize) {
			// If the image is small, we link directly to the image.
			preview = append(preview,
				fmt.Sprintf("https://upload.wikimedia.org/wikipedia/%s/%s/%s", fileSite, prefix, image.Name),
			)
		} else {
			width := previewSize
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
				// For some reason video files have an extra dash. It works also without,
				// but we want to match thumbnails generated by MediaWiki as close as possible
				// (to use any cache which might exist there).
				if strings.HasPrefix(mediaType, "video/") {
					extraDash = "-"
				}
				if mediaType == "image/tiff" {
					// For some reason image/tiff files have "lossy" prefix. It works also without,
					// but we want to match thumbnails generated by MediaWiki as close as possible
					// (to use any cache which might exist there). We reuse pagePrefix for this.
					// TODO: Figure out when it is "lossy" with .jpg and "lossless" with .png extension.
					pagePrefix = "lossy-" + pagePrefix
				}
				thumbName := image.Name
				if len(thumbName) > longFilename {
					// Too long names are shortened. It works also with the long name, but we want
					// to match thumbnails generated by MediaWiki as close as possible (to use
					// any cache which might exist there).
					ext := strings.ToLower(path.Ext(thumbName))
					if ext == "" || ext == "." {
						thumbName = "thumbnail"
					} else {
						thumbName = "thumbnail" + ext
					}
				}
				preview = append(preview,
					fmt.Sprintf("https://upload.wikimedia.org/wikipedia/%s/thumb/%s/%s/%s%dpx-%s%s%s", fileSite, prefix, image.Name, pagePrefix, width, extraDash, thumbName, extraExtension),
				)
			}
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
						ID:         search.GetID(namespace, image.Name, mnemonicPrefix+"_FILE_NAME", 0),
						Confidence: highConfidence,
					},
					Prop:       search.GetStandardPropertyReference(mnemonicPrefix + "_FILE_NAME"),
					Identifier: image.Name,
				},
			},
			Reference: search.ReferenceClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, image.Name, mnemonicPrefix+"_FILE", 0),
						Confidence: highConfidence,
					},
					Prop: search.GetStandardPropertyReference(mnemonicPrefix + "_FILE"),
					IRI:  fmt.Sprintf("https://%s/wiki/File:%s", fileDomain, image.Name),
				},
			},
			Is: search.IsClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, image.Name, "FILE", 0),
						Confidence: highConfidence,
					},
					Prop: search.GetStandardPropertyReference("FILE"),
				},
			},
			File: search.FileClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, image.Name, "DATA", 0),
						Confidence: highConfidence,
					},
					Prop:    search.GetStandardPropertyReference("DATA"),
					Type:    mediaType,
					URL:     fmt.Sprintf("https://upload.wikimedia.org/wikipedia/%s/%s/%s", fileSite, prefix, image.Name),
					Preview: preview,
				},
			},
			String: search.StringClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, image.Name, "MEDIA_TYPE", 0),
						Confidence: highConfidence,
					},
					Prop:   search.GetStandardPropertyReference("MEDIA_TYPE"),
					String: mediaType,
				},
			},
			Amount: search.AmountClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, image.Name, "SIZE", 0),
						Confidence: highConfidence,
					},
					Prop:   search.GetStandardPropertyReference("SIZE"),
					Amount: float64(image.Size),
					Unit:   search.AmountUnitByte,
				},
			},
			Enumeration: search.EnumerationClaims{
				{
					CoreClaim: search.CoreClaim{
						ID:         search.GetID(namespace, image.Name, "MEDIAWIKI_MEDIA_TYPE", 0),
						Confidence: highConfidence,
					},
					Prop: search.GetStandardPropertyReference("MEDIAWIKI_MEDIA_TYPE"),
					Enum: []string{strings.ToLower(image.MediaType)},
				},
			},
		},
	}

	if pageCount > 0 {
		document.Active.Amount = append(document.Active.Amount, search.AmountClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, image.Name, "PAGE_COUNT", 0),
				Confidence: mediumConfidence,
			},
			Prop:   search.GetStandardPropertyReference("PAGE_COUNT"),
			Amount: float64(pageCount),
			Unit:   search.AmountUnitNone,
		})
	}
	if duration > 0.0 {
		document.Active.Amount = append(document.Active.Amount, search.AmountClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, image.Name, "LENGTH", 0),
				Confidence: mediumConfidence,
			},
			Prop:   search.GetStandardPropertyReference("LENGTH"),
			Amount: duration,
			Unit:   search.AmountUnitSecond,
		})
	}
	if image.Width > 0 && image.Height > 0 {
		document.Active.Amount = append(document.Active.Amount, search.AmountClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, image.Name, "WIDTH", 0),
				Confidence: mediumConfidence,
			},
			Prop:   search.GetStandardPropertyReference("WIDTH"),
			Amount: float64(image.Width),
			Unit:   search.AmountUnitPixel,
		}, search.AmountClaim{
			CoreClaim: search.CoreClaim{
				ID:         search.GetID(namespace, image.Name, "HEIGHT", 0),
				Confidence: mediumConfidence,
			},
			Prop:   search.GetStandardPropertyReference("HEIGHT"),
			Amount: float64(image.Height),
			Unit:   search.AmountUnitPixel,
		})
	}

	return &document, nil
}
