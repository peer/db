package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
)

var (
	extensionToMediaTypes = map[string][]string{
		".djvu": []string{"image/vnd.djvu"},
		".pdf":  []string{"application/pdf"},
		".stl":  []string{"application/sla"},
		// We have to additionally determine which of these two media types a file is.
		".webm": []string{"audio/webm", "video/webm"},
		".mpg":  []string{"video/mpeg"},
		// Wikimedia Commons uses "application/ogg" for both "ogv" and "oga", but we find it more informative
		// to tell if it is audio or video through the media type, if this information is already available.
		".ogv":  []string{"video/ogg"},
		".ogg":  []string{"audio/ogg"},
		".oga":  []string{"audio/ogg"},
		".opus": []string{"audio/ogg"},
		".mid":  []string{"audio/midi"},
		".flac": []string{"audio/flac"},
		".wav":  []string{"audio/wav"},
		".mp3":  []string{"audio/mpeg"},
		".tiff": []string{"image/tiff"},
		".tif":  []string{"image/tiff"},
		".png":  []string{"image/png"},
		".gif":  []string{"image/gif"},
		".jpg":  []string{"image/jpeg"},
		".jpeg": []string{"image/jpeg"},
		".webp": []string{"image/webp"},
		".xcf":  []string{"image/x-xcf"},
		".svg":  []string{"image/svg+xml"},
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

type FileInfo struct {
	MediaType string
	PageURL   string
	URL       string
	Preview   []string
}

type ImageInfo struct {
	Mime string `json:"mime"`
	// Additional fields for "size|bitdepth":
	// Size      int     `json:"size"`
	// Width              int     `json:"width"`
	// Height             int     `json:"height"`
	// PageCount int     `json:"pagecount"`
	// Duration  float64 `json:"duration"`
	// BitDepth  int     `json:"bitdepth"`
}

type APIResponse struct {
	BatchComplete string `json:"batchcomplete"`
	Continue      struct {
		IIStart  string `json:"iistart"`
		Continue string `json:"continue"`
	} `json:"continue"`
	Query struct {
		Pages map[string]struct {
			PageID          int         `json:"pageid"`
			Namespace       int         `json:"ns"`
			Title           string      `json:"title"`
			ImageRepository string      `json:"imagerepository"`
			ImageInfo       []ImageInfo `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
}

func getWikimediaCommonsFilePrefix(filename string) string {
	sum := md5.Sum([]byte(filename))
	digest := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s/%s", digest[0:1], digest[0:2])
}

func makeFileInfo(mediaType, filename string) FileInfo {
	prefix := getWikimediaCommonsFilePrefix(filename)
	preview := []string{}
	if !noPreview[mediaType] {
		pagePrefix := ""
		if hasPages[mediaType] {
			pagePrefix = "page1-"
		}
		extraExtension := ""
		if thumbnailExtraExtensions[mediaType] != "" {
			extraExtension = thumbnailExtraExtensions[mediaType]
		}
		preview = []string{
			fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/thumb/%s/%s/%s128px-%s%s", prefix, filename, pagePrefix, filename, extraExtension),
		}
	}
	return FileInfo{
		MediaType: mediaType,
		PageURL:   fmt.Sprintf("https://commons.wikimedia.org/wiki/File:%s", filename),
		URL:       fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/%s/%s", prefix, filename),
		Preview:   preview,
	}
}

func getFileInfo(ctx context.Context, title string) (FileInfo, errors.E) {
	titleWithPrefix := "File:" + title
	filename := strings.ReplaceAll(title, " ", "_")
	extension := strings.ToLower(path.Ext(title))
	mediaTypes := extensionToMediaTypes[extension]
	if len(mediaTypes) == 0 {
		return FileInfo{}, nil
	} else if len(mediaTypes) == 1 {
		return makeFileInfo(mediaTypes[0], filename), nil
	}

	// We have to use the API to determine the media type.
	// TODO: Fetch and use also other image info data using "size|bitdepth|extmetadata|metadata|commonmetadata".
	//       Check out also "iiextmetadatamultilang" and "iimetadataversion".
	u := fmt.Sprintf(
		"https://commons.wikimedia.org/w/api.php?action=query&prop=imageinfo&iiprop=mime&titles=%s&format=json",
		url.QueryEscape(titleWithPrefix),
	)
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return FileInfo{}, errors.WithStack(err)
	}
	// TODO: Make contact e-mail into a CLI argument.
	userAgent := fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision)
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return FileInfo{}, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return FileInfo{}, errors.Errorf(`bad response status (%s) for "%s": %s`, resp.Status, titleWithPrefix, strings.TrimSpace(string(body)))
	}

	var apiResponse APIResponse
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&apiResponse)
	if err != nil {
		return FileInfo{}, errors.WithMessagef(err, `json decode failure for "%s"`, titleWithPrefix)
	}

	if len(apiResponse.Query.Pages) != 1 {
		return FileInfo{}, errors.Errorf(`not exactly one result page for "%s"`, titleWithPrefix)
	}

	for _, page := range apiResponse.Query.Pages {
		if page.Title != titleWithPrefix {
			return FileInfo{}, errors.Errorf(`result title "%s" does not match query title "%s"`, page.Title, titleWithPrefix)
		}
		if len(page.ImageInfo) != 1 {
			return FileInfo{}, errors.Errorf(`not exactly one image info result for "%s"`, titleWithPrefix)
		}
		return makeFileInfo(page.ImageInfo[0].Mime, filename), nil
	}

	// It cannot really get there, but to make compiler happy.
	return FileInfo{}, errors.Errorf(`not exactly one result page for "%s"`, titleWithPrefix)
}
