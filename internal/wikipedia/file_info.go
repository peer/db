package wikipedia

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/time/rate"
)

type ImageInfo struct {
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
	Known           bool        `json:"known"`
	Invalid         bool        `json:"invalid"`
	InvalidReason   string      `json:"invalidreason"`
	ImageRepository string      `json:"imagerepository"`
	ImageInfo       []ImageInfo `json:"imageinfo"`
}

type apiResponse struct {
	Error         json.RawMessage `json:"error,omitempty"`
	ServedBy      string          `json:"servedby,omitempty"`
	BatchComplete bool            `json:"batchcomplete"`
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
	ImageInfoChan chan<- ImageInfo
	ErrChan       chan<- errors.E
}

// apiWorkersPerSite is a map between a site and another map, which is a map between a context and a channel.
var apiWorkersPerSite sync.Map

func doAPIRequest(ctx context.Context, httpClient *retryablehttp.Client, site, token string, tasks []apiTask) errors.E {
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
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(encodedData))
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = debugURL
		return errE
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(encodedData)))
	if token != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}
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
	if apiResp.Error != nil {
		errE := errors.New("response error")
		errors.Details(errE)["url"] = debugURL
		errors.Details(errE)["body"] = apiResp.Error
		return errE
	}

	redirects := map[string]string{}
	redirectsReverse := map[string][]string{}
	for _, redirect := range apiResp.Query.Redirects {
		redirects[redirect.From] = redirect.To
		if _, ok := redirectsReverse[redirect.To]; !ok {
			redirectsReverse[redirect.To] = []string{}
		}
		redirectsReverse[redirect.To] = append(redirectsReverse[redirect.To], redirect.From)
	}

	pagesMap := map[string]page{}
	for _, page := range apiResp.Query.Pages {
		redirects, ok := redirectsReverse[page.Title]
		if !ok {
			// Fake redirect.
			redirects = []string{page.Title}
		}
		for _, redirect := range redirects {
			// Make a copy.
			p := page
			// This assignment is unnecessary for a fake redirect.
			p.Title = redirect
			if _, ok := tasksMap[p.Title]; !ok {
				errE := errors.New("unexpected result page")
				errors.Details(errE)["got"] = p.Title
				titles := []string{}
				for t := range tasksMap {
					titles = append(titles, t)
				}
				sort.Strings(titles)
				errors.Details(errE)["expected"] = titles
				errors.Details(errE)["url"] = debugURL
				return errE
			}
			pagesMap[p.Title] = p
		}
	}

	if len(tasksMap) != len(pagesMap) {
		errE := errors.New("unexpected result page(s)")
		errors.Details(errE)["got"] = len(pagesMap)
		errors.Details(errE)["expected"] = len(tasksMap)
		errors.Details(errE)["url"] = debugURL
		return errE
	}

	// Now we report errors only to individual tasks.
	// Once we get to here all tasks have to be processed.
	for _, page := range pagesMap {
		// We have checked above that tasks per page always exists.
		pageTasks := tasksMap[page.Title]
		if page.Missing && !page.Known {
			for _, task := range pageTasks {
				errE := errors.WithStack(NotFoundError)
				errors.Details(errE)["title"] = page.Title
				task.ErrChan <- errE
			}
		} else if page.Invalid {
			for _, task := range pageTasks {
				errE := errors.New("invalid")
				errors.Details(errE)["title"] = page.Title
				errors.Details(errE)["reason"] = page.InvalidReason
				errors.Details(errE)["url"] = debugURL
				task.ErrChan <- errE
			}
		} else if len(page.ImageInfo) == 0 {
			for _, task := range pageTasks {
				ii := ImageInfo{}
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
				errors.Details(errE)["url"] = debugURL
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
func getAPIWorker(ctx context.Context, httpClient *retryablehttp.Client, site, token string, apiLimit int) chan<- apiTask {
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

				errE := doAPIRequest(ctx, httpClient, site, token, tasks)
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

func getImageInfoChan(ctx context.Context, httpClient *retryablehttp.Client, site, token string, apiLimit int, title string) (<-chan ImageInfo, <-chan errors.E) {
	apiTaskChan := getAPIWorker(ctx, httpClient, site, token, apiLimit)

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

func getImageInfoForFilename(ctx context.Context, httpClient *retryablehttp.Client, site, token string, apiLimit int, filename string) (ImageInfo, errors.E) {
	// First we make sure we do not have underscores.
	title := strings.ReplaceAll(filename, "_", " ")
	// The first letter has to be upper case.
	title = FirstUpperCase(title)
	title = "File:" + title

	ii, err := GetImageInfo(ctx, httpClient, site, token, apiLimit, title)
	if err != nil {
		errors.Details(err)["file"] = filename
	}
	return ii, err
}

func GetImageInfo(ctx context.Context, httpClient *retryablehttp.Client, site, token string, apiLimit int, title string) (ImageInfo, errors.E) {
	imageInfoChan, errChan := getImageInfoChan(ctx, httpClient, site, token, apiLimit, title)

	for {
		select {
		case <-ctx.Done():
			return ImageInfo{}, errors.WithStack(ctx.Err())
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
			return ImageInfo{}, err
		}
	}
}
