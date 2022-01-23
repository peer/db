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
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/time/rate"
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

type Page struct {
	PageID          int         `json:"pageid"`
	Namespace       int         `json:"ns"`
	Title           string      `json:"title"`
	Missing         bool        `json:"missing"`
	Invalid         bool        `json:"invalid"`
	InvalidReason   string      `json:"invalidreason"`
	ImageRepository string      `json:"imagerepository"`
	ImageInfo       []ImageInfo `json:"imageinfo"`
}

type APIResponse struct {
	BatchComplete bool `json:"batchcomplete"`
	Continue      struct {
		IIStart  string `json:"iistart"`
		Continue string `json:"continue"`
	} `json:"continue"`
	Query struct {
		Pages []Page `json:"pages"`
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

type apiTask struct {
	Title         string
	ImageInfoChan chan<- ImageInfo
	ErrChan       chan<- errors.E
}

var apiWorkers sync.Map

func doAPIRequest(ctx context.Context, tasks []apiTask) errors.E {
	titles := strings.Builder{}
	tasksMap := map[string][]apiTask{}
	for _, task := range tasks {
		titleWithPrefix := "File:" + task.Title
		if _, ok := tasksMap[titleWithPrefix]; ok {
			tasksMap[titleWithPrefix] = append(tasksMap[titleWithPrefix], task)
		} else {
			tasksMap[titleWithPrefix] = []apiTask{task}
			// Separator, instead of "|". It has also be the prefix.
			titles.WriteString("%1F")
			titles.WriteString(url.QueryEscape(titleWithPrefix))
		}
	}

	// TODO: Fetch and use also other image info data using "size|bitdepth|extmetadata|metadata|commonmetadata".
	//       Check out also "iiextmetadatamultilang" and "iimetadataversion".
	u := fmt.Sprintf(
		"https://commons.wikimedia.org/w/api.php?action=query&prop=imageinfo&iiprop=mime&titles=%s&format=json&formatversion=2",
		titles.String(),
	)
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	// TODO: Make contact e-mail into a CLI argument.
	userAgent := fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision)
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return errors.WithMessage(err, u)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.Errorf(`%s: bad response status (%s): %s`, u, resp.Status, strings.TrimSpace(string(body)))
	}

	var apiResponse APIResponse
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&apiResponse)
	if err != nil {
		return errors.WithMessagef(err, `%s: json decode failure`, u)
	}

	if len(apiResponse.Query.Pages) != len(tasksMap) {
		return errors.Errorf(`got %d result page(s), expected %d`, len(apiResponse.Query.Pages), len(tasksMap))
	}

	pagesMap := map[string]Page{}
	for _, page := range apiResponse.Query.Pages {
		if _, ok := tasksMap[page.Title]; !ok {
			return errors.Errorf(`unexpected result page for "%s"`, page.Title)
		}
		pagesMap[page.Title] = page
	}

	if len(tasksMap) != len(pagesMap) {
		return errors.Errorf(`got %d unique result page(s), expected %d`, len(pagesMap), len(tasksMap))
	}

	// Now we report errors only to individual tasks. Once we get to here all tasks
	// have to be processed and all their channels closed.
	for _, page := range pagesMap {
		// We have checked above that tasks per page always exists.
		pageTasks := tasksMap[page.Title]
		if page.Missing {
			for _, task := range pageTasks {
				task.ErrChan <- errors.Errorf(`"%s" missing`, page.Title)
			}
		} else if page.Invalid {
			for _, task := range pageTasks {
				task.ErrChan <- errors.Errorf(`"%s" invalid: %s`, page.Title, page.InvalidReason)
			}
		} else if len(page.ImageInfo) != 1 {
			for _, task := range pageTasks {
				task.ErrChan <- errors.Errorf(`not exactly one image info result for "%s"`, page.Title)
			}
		} else {
			for _, task := range pageTasks {
				task.ImageInfoChan <- page.ImageInfo[0]
			}
		}
		for _, task := range pageTasks {
			close(task.ImageInfoChan)
			close(task.ErrChan)
		}
	}

	return nil
}

func getAPIWorker(ctx context.Context) chan<- apiTask {
	// Sanity check so that we do not do unnecessary work of setup
	// just to be cleaned up soon aftwards.
	if ctx.Err() != nil {
		return nil
	}

	// A queue of up to (and including) 50 tasks.
	// 50 is the limit per one API call (500 for clients allowed higher limits).
	apiTaskChan := make(chan apiTask, 50)

	existingApiTaskChan, loaded := apiWorkers.LoadOrStore(ctx, apiTaskChan)
	if loaded {
		// We made it just in case but we do not need it.
		close(apiTaskChan)
		return existingApiTaskChan.(chan apiTask)
	}

	go func() {
		tasks := []apiTask{}
		limiter := rate.NewLimiter(rate.Every(time.Second), 1)

		defer func() {
			if ctx.Err() == nil {
				// We have a problem, we are here but context has not been canceled.
				// Is this a panic? For now we do not do anything and just let it propagate.
				// TODO: Can we do something better?
				return
			}

			// First we delete the worker so that it is not available anymore for this context.
			apiWorkers.Delete(ctx)

			for {
				// There might be pending tasks for which we should close channels.
				for _, task := range tasks {
					close(task.ImageInfoChan)
					close(task.ErrChan)
				}
				tasks = []apiTask{}

				// Allow other goroutines to send their tasks, if they are any still in flight.
				runtime.Gosched()

				// There might be more tasks in the queue, drain it.
			DRAIN:
				select {
				case task := <-apiTaskChan:
					tasks = append(tasks, task)
				default:
					break DRAIN
				}

				if len(tasks) == 0 {
					break
				}
			}

			// TODO: Is it really safe to close the channel now?
			close(apiTaskChan)
		}()

		for {
			select {
			// Wait for at least one task to be available.
			case task := <-apiTaskChan:
				tasks = []apiTask{task}
				for {
					// Make sure we are respecting the rate limit.
					err := limiter.Wait(ctx)
					if err != nil {
						// Context has been canceled.
						return
					}

					// Drain any other pending task, up to 50.
				DRAIN:
					for len(tasks) < 50 {
						select {
						case task := <-apiTaskChan:
							tasks = append(tasks, task)
						default:
							break DRAIN
						}
					}

					errE := doAPIRequest(ctx, tasks)
					if errE == nil {
						// No error, we exit the retry loop.
						tasks = []apiTask{}
						break
					}

					if errors.Is(errE, context.Canceled) || errors.Is(errE, context.DeadlineExceeded) {
						// Context has been canceled.
						return
					}

					// TODO: Use logger.
					fmt.Fprintf(os.Stderr, "API request failed: %+v\n", errE)
					// We retry here.
				}
			case <-ctx.Done():
				// Context has been canceled.
				return
			}
		}
	}()

	return apiTaskChan
}

func getImageInfo(ctx context.Context, title string) (<-chan ImageInfo, <-chan errors.E) {
	apiTaskChan := getAPIWorker(ctx)

	imageInfoChan := make(chan ImageInfo)
	errChan := make(chan errors.E)

	select {
	case <-ctx.Done():
		close(imageInfoChan)
		close(errChan)
		return nil, nil
	case apiTaskChan <- apiTask{
		Title:         title,
		ImageInfoChan: imageInfoChan,
		ErrChan:       errChan,
	}:
		return imageInfoChan, errChan
	}
}

func getFileInfo(ctx context.Context, title string) (FileInfo, errors.E) {
	filename := strings.ReplaceAll(title, " ", "_")
	extension := strings.ToLower(path.Ext(title))
	mediaTypes := extensionToMediaTypes[extension]
	if len(mediaTypes) == 0 {
		return FileInfo{}, nil
	} else if len(mediaTypes) == 1 {
		return makeFileInfo(mediaTypes[0], filename), nil
	}

	// We have to use the API to determine the media type.
	imageInfoChan, errChan := getImageInfo(ctx, title)

	for {
		select {
		case <-ctx.Done():
			return FileInfo{}, errors.WithStack(ctx.Err())
		case imageInfo, ok := <-imageInfoChan:
			if !ok {
				imageInfoChan = nil
				// Break the select and retry the loop.
				break
			}
			return makeFileInfo(imageInfo.Mime, filename), nil
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				// Break the select and retry the loop.
				break
			}
			return FileInfo{}, err
		}
	}
}
