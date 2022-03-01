package wikipedia

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/time/rate"
)

const (
	// A queue of up to (and including) 50 tasks.
	// 50 is the limit per one API call (500 for clients allowed higher limits).
	apiLimit = 50
)

type imageInfo struct {
	Mime                string  `json:"mime"`
	Size                int     `json:"size"`
	Width               int     `json:"width"`
	Height              int     `json:"height"`
	PageCount           int     `json:"pagecount"`
	Duration            float64 `json:"duration"`
	URL                 string  `json:"url"`
	DescriptionURL      string  `json:"descriptionurl"`
	DescriptionShortURL string  `json:"descriptionshorturl"`
	// Set if the requested page redirected to another page and info is from that other page.
	Redirect string `json:"-"`
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
		// We on purpose do not list "normalized" field and we want response parsing to fail
		// if one is included: we want to always pass correctly normalized titles ourselves.
		Redirects []struct {
			From       string `json:"from"`
			To         string `json:"to"`
			ToFragment string `json:"tofragment,omitempty"`
		} `json:"redirects"`
		Pages []page `json:"pages"`
	} `json:"query"`
}

type apiTask struct {
	Title         string
	ImageInfoChan chan<- imageInfo
	ErrChan       chan<- errors.E
}

// apiWorkersPerSite is a map between a site and another map, which is a map between a context and a channel.
var apiWorkersPerSite sync.Map

func doAPIRequest(ctx context.Context, httpClient *retryablehttp.Client, site string, tasks []apiTask) errors.E {
	titles := strings.Builder{}
	tasksMap := map[string][]apiTask{}
	for _, task := range tasks {
		if _, ok := tasksMap[task.Title]; ok {
			tasksMap[task.Title] = append(tasksMap[task.Title], task)
		} else {
			tasksMap[task.Title] = []apiTask{task}
			// Separator, instead of "|". It has also be the prefix.
			titles.WriteString("\u001F")
			titles.WriteString(task.Title)
		}
	}

	data := url.Values{}
	data.Set("action", "query")
	data.Set("prop", "imageinfo")
	// TODO: Fetch and use also other image info data using "bitdepth|extmetadata|metadata|commonmetadata".
	//       Check out also "iiextmetadatamultilang" and "iimetadataversion".
	data.Set("iiprop", "mime|size|url")
	data.Set("format", "json")
	data.Set("formatversion", "2")
	data.Set("titles", titles.String())
	data.Set("redirects", "")
	encodedData := data.Encode()
	apiURL := fmt.Sprintf("https://%s/w/api.php", site)
	debugURL := fmt.Sprintf("%s?%s", apiURL, encodedData)
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodPost, "https://commons.wikimedia.org/w/api.php", strings.NewReader(encodedData))
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = debugURL
		return errE
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(encodedData)))
	resp, err := httpClient.Do(req)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = debugURL
		return errE
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errE := errors.New("bad response status")
		errors.Details(errE)["url"] = debugURL
		errors.Details(errE)["code"] = resp.StatusCode
		errors.Details(errE)["body"] = strings.TrimSpace(string(body))
		return errE
	}

	var apiResp apiResponse
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&apiResp)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = debugURL
		return errE
	}

	if len(apiResp.Query.Pages) != len(tasksMap) {
		errE := errors.New("unexpected result page(s)")
		errors.Details(errE)["got"] = len(apiResp.Query.Pages)
		errors.Details(errE)["expected"] = len(tasksMap)
		return errE
	}

	redirects := map[string]string{}
	redirectsReverse := map[string]string{}
	for _, redirect := range apiResp.Query.Redirects {
		redirects[redirect.From] = redirect.To
		redirectsReverse[redirect.To] = redirect.From
	}

	pagesMap := map[string]page{}
	for _, page := range apiResp.Query.Pages {
		if redirect, ok := redirectsReverse[page.Title]; ok {
			page.Title = redirect
		}
		if _, ok := tasksMap[page.Title]; !ok {
			errE := errors.New("unexpected result page")
			errors.Details(errE)["title"] = page.Title
			return errE
		}
		pagesMap[page.Title] = page
	}

	if len(tasksMap) != len(pagesMap) {
		errE := errors.New("unexpected mapped result page(s)")
		errors.Details(errE)["got"] = len(pagesMap)
		errors.Details(errE)["expected"] = len(tasksMap)
		return errE
	}

	// Now we report errors only to individual tasks.
	// Once we get to here all tasks have to be processed.
	for _, page := range pagesMap {
		// We have checked above that tasks per page always exists.
		pageTasks := tasksMap[page.Title]
		if page.Missing {
			for _, task := range pageTasks {
				errE := errors.New("missing")
				errors.Details(errE)["title"] = page.Title
				task.ErrChan <- errE
			}
		} else if page.Invalid {
			for _, task := range pageTasks {
				errE := errors.New("invalid")
				errors.Details(errE)["title"] = page.Title
				errors.Details(errE)["reason"] = page.InvalidReason
				task.ErrChan <- errE
			}
		} else if len(page.ImageInfo) == 0 {
			for _, task := range pageTasks {
				ii := imageInfo{}
				// Set redirect if there is one, otherwise this sets an empty string.
				ii.Redirect = redirects[page.Title]
				ii.Redirect = strings.TrimPrefix(ii.Redirect, "File:")
				ii.Redirect = strings.ReplaceAll(ii.Redirect, " ", "_")
				task.ImageInfoChan <- ii
			}
		} else if len(page.ImageInfo) > 1 {
			for _, task := range pageTasks {
				errE := errors.New("more than one image info result")
				errors.Details(errE)["title"] = page.Title
				task.ErrChan <- errE
			}
		} else {
			for _, task := range pageTasks {
				// Make a copy.
				ii := page.ImageInfo[0]
				// Set redirect if there is one, otherwise this sets an empty string.
				ii.Redirect = redirects[page.Title]
				ii.Redirect = strings.TrimPrefix(ii.Redirect, "File:")
				ii.Redirect = strings.ReplaceAll(ii.Redirect, " ", "_")
				task.ImageInfoChan <- ii
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
func getAPIWorker(ctx context.Context, httpClient *retryablehttp.Client, site string) chan<- apiTask {
	// Sanity check so that we do not do unnecessary work of setup
	// just to be cleaned up soon aftwards.
	if ctx.Err() != nil {
		return nil
	}

	apiWorkersInterface, _ := apiWorkersPerSite.LoadOrStore(site, &sync.Map{})
	apiWorkers := apiWorkersInterface.(*sync.Map) //nolint: errcheck

	apiTaskChan := make(chan apiTask, apiLimit)

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
				// Make sure we are respecting the rate limit.
				err := limiter.Wait(ctx)
				if err != nil {
					// Context has been canceled.
					return
				}

				// Drain any other pending task, up to apiLimit.
			DRAIN:
				for len(tasks) < apiLimit {
					select {
					case task := <-apiTaskChan:
						tasks = append(tasks, task)
					default:
						break DRAIN
					}
				}

				errE := doAPIRequest(ctx, httpClient, site, tasks)
				if errE == nil {
					// No error, we continue the outer loop.
					continue
				}

				if errors.Is(errE, context.Canceled) || errors.Is(errE, context.DeadlineExceeded) {
					// Context has been canceled.
					return
				}

				// We report the error.
				errE = errors.Errorf("API request failed: %w", errE)
				for _, t := range tasks {
					t.ErrChan <- errE
				}
			case <-ctx.Done():
				// Context has been canceled.
				return
			}
		}
	}()

	return apiTaskChan
}

func getImageInfoChan(ctx context.Context, httpClient *retryablehttp.Client, site, title string) (<-chan imageInfo, <-chan errors.E) {
	apiTaskChan := getAPIWorker(ctx, httpClient, site)

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

// Implementation changes case only of ASCII characters. Using unicode.ToUpper sometimes
// changes case of characters for which Mediawiki does not change it. If we do change case when
// Mediawiki does not a corresponding file is not found. On the other hand, if we do not change
// case when Mediawiki does, then API returns a "normalized" field which fails JSON decoding
// so we detect such cases, if and when they happen.
// See: https://phabricator.wikimedia.org/T301758
func FirstUpperCase(str string) string {
	runes := []rune(str)
	r := runes[0]
	if r <= unicode.MaxASCII {
		if 'a' <= r && r <= 'z' {
			r -= 'a' - 'A'
		}
	}
	runes[0] = r
	return string(runes)
}

func getImageInfo(ctx context.Context, httpClient *retryablehttp.Client, site, filename string) (imageInfo, errors.E) {
	// First we make sure we do not have underscores.
	title := strings.ReplaceAll(filename, "_", " ")
	// The first letter has to be upper case.
	title = FirstUpperCase(title)
	title = "File:" + title

	imageInfoChan, errChan := getImageInfoChan(ctx, httpClient, site, title)

	for {
		select {
		case <-ctx.Done():
			return imageInfo{}, errors.WithStack(ctx.Err())
		case info, ok := <-imageInfoChan:
			if !ok {
				imageInfoChan = nil
				// Break the select and retry the loop.
				break
			}
			return info, nil
		case err, ok := <-errChan:
			if !ok {
				errChan = nil
				// Break the select and retry the loop.
				break
			}
			return imageInfo{}, err
		}
	}
}
