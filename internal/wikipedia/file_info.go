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
	Mime      string  `json:"mime"`
	Size      int     `json:"size"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	PageCount int     `json:"pagecount"`
	Duration  float64 `json:"duration"`
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
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"redirects"`
		Pages []page `json:"pages"`
	} `json:"query"`
}

type apiTask struct {
	Title         string
	ImageInfoChan chan<- imageInfo
	ErrChan       chan<- errors.E
}

var apiWorkers sync.Map

func doAPIRequest(ctx context.Context, httpClient *retryablehttp.Client, tasks []apiTask) errors.E {
	titles := strings.Builder{}
	tasksMap := map[string][]apiTask{}
	for _, task := range tasks {
		titleWithPrefix := "File:" + task.Title
		if _, ok := tasksMap[titleWithPrefix]; ok {
			tasksMap[titleWithPrefix] = append(tasksMap[titleWithPrefix], task)
		} else {
			tasksMap[titleWithPrefix] = []apiTask{task}
			// Separator, instead of "|". It has also be the prefix.
			titles.WriteString("\u001F")
			titles.WriteString(titleWithPrefix)
		}
	}

	data := url.Values{}
	data.Set("action", "query")
	data.Set("prop", "imageinfo")
	// TODO: Fetch and use also other image info data using "bitdepth|extmetadata|metadata|commonmetadata".
	//       Check out also "iiextmetadatamultilang" and "iimetadataversion".
	data.Set("iiprop", "mime|size")
	data.Set("format", "json")
	data.Set("formatversion", "2")
	data.Set("titles", titles.String())
	data.Set("redirects", "")
	encodedData := data.Encode()
	debugURL := fmt.Sprintf("https://commons.wikimedia.org/w/api.php?%s", encodedData)
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodPost, "https://commons.wikimedia.org/w/api.php", strings.NewReader(encodedData))
	if err != nil {
		return errors.WithStack(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(encodedData)))
	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.WithMessage(err, debugURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.Errorf(`%s: bad response status (%s): %s`, debugURL, resp.Status, strings.TrimSpace(string(body)))
	}

	var apiResp apiResponse
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&apiResp)
	if err != nil {
		return errors.WithMessagef(err, `%s: json decode failure`, debugURL)
	}

	if len(apiResp.Query.Pages) != len(tasksMap) {
		return errors.Errorf(`got %d result page(s), expected %d`, len(apiResp.Query.Pages), len(tasksMap))
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
func getAPIWorker(ctx context.Context, httpClient *retryablehttp.Client) chan<- apiTask {
	// Sanity check so that we do not do unnecessary work of setup
	// just to be cleaned up soon aftwards.
	if ctx.Err() != nil {
		return nil
	}

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

				errE := doAPIRequest(ctx, httpClient, tasks)
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

func getImageInfoChan(ctx context.Context, httpClient *retryablehttp.Client, title string) (<-chan imageInfo, <-chan errors.E) {
	apiTaskChan := getAPIWorker(ctx, httpClient)

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
func firstUpperCase(str string) string {
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

func getImageInfo(ctx context.Context, httpClient *retryablehttp.Client, filename string) (imageInfo, errors.E) {
	// First we make sure we do not have underscores.
	title := strings.ReplaceAll(filename, "_", " ")
	// The first letter has to be upper case.
	title = firstUpperCase(title)

	imageInfoChan, errChan := getImageInfoChan(ctx, httpClient, title)

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
