package wikipedia

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
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/time/rate"
)

var (
	extensionToMediaTypes = map[string][]string{
		".djvu": {"image/vnd.djvu"},
		".pdf":  {"application/pdf"},
		".stl":  {"application/sla"},
		// We have to additionally determine which of these two media types a file is.
		".webm": {"audio/webm", "video/webm"},
		".mpg":  {"video/mpeg"},
		// Wikimedia Commons uses "application/ogg" for both "ogv" and "oga", but we find it more informative
		// to tell if it is audio or video through the media type, if this information is already available.
		".ogv":  {"video/ogg"},
		".ogg":  {"audio/ogg"},
		".oga":  {"audio/ogg"},
		".opus": {"audio/ogg"},
		".mid":  {"audio/midi"},
		".midi": {"audio/midi"},
		".flac": {"audio/flac"},
		".wav":  {"audio/wav"},
		".mp3":  {"audio/mpeg"},
		".tiff": {"image/tiff"},
		".tif":  {"image/tiff"},
		".png":  {"image/png"},
		".gif":  {"image/gif"},
		".jpg":  {"image/jpeg"},
		".jpeg": {"image/jpeg"},
		".webp": {"image/webp"},
		".xcf":  {"image/x-xcf"},
		".svg":  {"image/svg+xml"},
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

type imageInfo struct {
	Mime      string  `json:"mime"`
	Size      int     `json:"size"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	PageCount int     `json:"pagecount"`
	Duration  float64 `json:"duration"`
}

type page struct {
	PageID          int         `json:"pageid"`
	Namespace       int         `json:"ns"`
	Title           string      `json:"title"`
	Missing         bool        `json:"missing"`
	Invalid         bool        `json:"invalid"`
	InvalidReason   string      `json:"invalidreason"`
	ImageRepository string      `json:"imagerepository"`
	ImageInfo       []imageInfo `json:"imageinfo"`
}

type apiResponse struct {
	BatchComplete bool `json:"batchcomplete"`
	Continue      struct {
		IIStart  string `json:"iistart"`
		Continue string `json:"continue"`
	} `json:"continue"`
	Query struct {
		Pages []page `json:"pages"`
	} `json:"query"`
}

func getWikimediaCommonsFilePrefix(filename string) string {
	sum := md5.Sum([]byte(filename))
	digest := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s/%s", digest[0:1], digest[0:2])
}

func makeFileInfo(info imageInfo, filename string) FileInfo {
	prefix := getWikimediaCommonsFilePrefix(filename)
	pages := info.PageCount
	if pages == 0 {
		pages = 1
	}
	preview := []string{}
	if !noPreview[info.Mime] {
		for page := 1; page <= pages; page++ {
			pagePrefix := ""
			if hasPages[info.Mime] {
				pagePrefix = fmt.Sprintf("page%d-", page)
			}
			extraExtension := ""
			if thumbnailExtraExtensions[info.Mime] != "" {
				extraExtension = thumbnailExtraExtensions[info.Mime]
			}
			preview = append(preview,
				fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/thumb/%s/%s/%s256px-%s%s", prefix, filename, pagePrefix, filename, extraExtension),
			)
		}
	}
	return FileInfo{
		MediaType: info.Mime,
		PageURL:   fmt.Sprintf("https://commons.wikimedia.org/wiki/File:%s", filename),
		URL:       fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/%s/%s", prefix, filename),
		Preview:   preview,
	}
}

type apiTask struct {
	Title         string
	ImageInfoChan chan<- imageInfo
	ErrChan       chan<- errors.E
}

var apiWorkers sync.Map

func doAPIRequest(ctx context.Context, client *retryablehttp.Client, tasks []apiTask) errors.E {
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

	// TODO: Fetch and use also other image info data using "bitdepth|extmetadata|metadata|commonmetadata".
	//       Check out also "iiextmetadatamultilang" and "iimetadataversion".
	u := fmt.Sprintf(
		"https://commons.wikimedia.org/w/api.php?action=query&prop=imageinfo&iiprop=mime|size&titles=%s&format=json&formatversion=2",
		titles.String(),
	)
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.WithMessage(err, u)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.Errorf(`%s: bad response status (%s): %s`, u, resp.Status, strings.TrimSpace(string(body)))
	}

	var apiResp apiResponse
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&apiResp)
	if err != nil {
		return errors.WithMessagef(err, `%s: json decode failure`, u)
	}

	if len(apiResp.Query.Pages) != len(tasksMap) {
		return errors.Errorf(`got %d result page(s), expected %d`, len(apiResp.Query.Pages), len(tasksMap))
	}

	pagesMap := map[string]page{}
	for _, page := range apiResp.Query.Pages {
		if _, ok := tasksMap[page.Title]; !ok {
			return errors.Errorf(`unexpected result page for "%s"`, page.Title)
		}
		pagesMap[page.Title] = page
	}

	if len(tasksMap) != len(pagesMap) {
		return errors.Errorf(`got %d unique result page(s), expected %d`, len(pagesMap), len(tasksMap))
	}

	// Now we report errors only to individual tasks.
	// Once we get to here all tasks have to be processed.
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

// Returned apiTaskChan is never explicitly closed but it is left
// to the garbage collector to clean it up when it is suitable.
func getAPIWorker(ctx context.Context, client *retryablehttp.Client) chan<- apiTask {
	// Sanity check so that we do not do unnecessary work of setup
	// just to be cleaned up soon aftwards.
	if ctx.Err() != nil {
		return nil
	}

	// A queue of up to (and including) 50 tasks.
	// 50 is the limit per one API call (500 for clients allowed higher limits).
	apiTaskChan := make(chan apiTask, 50)

	existingAPITaskChan, loaded := apiWorkers.LoadOrStore(ctx, apiTaskChan)
	if loaded {
		// We made it just in case but we do not need it.
		close(apiTaskChan)
		return existingAPITaskChan.(chan apiTask)
	}

	go func() {
		defer apiWorkers.Delete(ctx)

		limiter := rate.NewLimiter(rate.Every(time.Second), 1)

		for {
			select {
			// Wait for at least one task to be available.
			case task := <-apiTaskChan:
				tasks := []apiTask{task}
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

					errE := doAPIRequest(ctx, client, tasks)
					if errE == nil {
						// No error, we exit the retry loop.
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

func getImageInfo(ctx context.Context, client *retryablehttp.Client, title string) (<-chan imageInfo, <-chan errors.E) {
	apiTaskChan := getAPIWorker(ctx, client)

	imageInfoChan := make(chan imageInfo)
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

func GetFileInfo(ctx context.Context, client *retryablehttp.Client, title string) (FileInfo, errors.E) {
	filename := strings.ReplaceAll(title, " ", "_")
	extension := strings.ToLower(path.Ext(title))
	mediaTypes := extensionToMediaTypes[extension]
	if len(mediaTypes) == 0 {
		return FileInfo{}, nil
	} else if len(mediaTypes) == 1 && !hasPages[mediaTypes[0]] {
		return makeFileInfo(imageInfo{Mime: mediaTypes[0]}, filename), nil
	}

	// We have to use the API to determine the media type or the number of pages.
	imageInfoChan, errChan := getImageInfo(ctx, client, title)

	for {
		select {
		case <-ctx.Done():
			return FileInfo{}, errors.WithStack(ctx.Err())
		case info, ok := <-imageInfoChan:
			if !ok {
				imageInfoChan = nil
				// Break the select and retry the loop.
				break
			}
			return makeFileInfo(info, filename), nil
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
