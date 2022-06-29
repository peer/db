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
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"golang.org/x/time/rate"
)

const (
	// All pages API has this limit and it does not depend on the token used.
	APILimit = 500
)

type PageReference struct {
	Identifier int64  `json:"pageid,omitempty"`
	Namespace  int    `json:"ns"`
	Title      string `json:"title"`
}

type AllPagesPage struct {
	Identifier int64             `json:"pageid"`
	Namespace  int               `json:"ns"`
	Title      string            `json:"title"`
	Properties map[string]string `json:"pageprops"`
	Categories []PageReference   `json:"categories,omitempty"`
	Templates  []PageReference   `json:"templates,omitempty"`
	Redirects  []PageReference   `json:"redirects,omitempty"`
}

type allPagesAPIResponse struct {
	Error         json.RawMessage   `json:"error,omitempty"`
	ServedBy      string            `json:"servedby,omitempty"`
	BatchComplete bool              `json:"batchcomplete"`
	Continue      map[string]string `json:"continue"`
	Query         struct {
		Pages []AllPagesPage `json:"pages"`
	} `json:"query"`
}

func ListAllPages(
	ctx context.Context, httpClient *retryablehttp.Client, namespaces []int, site, token string, limiter *rate.Limiter, output chan<- AllPagesPage,
) errors.E {
	// We still want to make sure we are contacting query API only once every second.
	localLimiter := rate.NewLimiter(rate.Every(time.Second), 1)

	for _, namespace := range namespaces {
		data := url.Values{}
		data.Set("action", "query")
		data.Set("format", "json")
		data.Set("formatversion", "2")
		data.Set("generator", "allpages")
		data.Set("gapnamespace", strconv.Itoa(namespace))
		data.Set("gapfilterredir", "nonredirects")
		data.Set("prop", "pageprops|categories|templates|redirects")
		data.Set("gaplimit", strconv.Itoa(APILimit))
		data.Set("cllimit", strconv.Itoa(APILimit))
		data.Set("tllimit", strconv.Itoa(APILimit))
		data.Set("rdlimit", strconv.Itoa(APILimit))

		var batch []AllPagesPage

		for {
			err := localLimiter.Wait(ctx)
			if err != nil {
				// Context has been canceled.
				return errors.WithStack(err)
			}

			err = limiter.Wait(ctx)
			if err != nil {
				// Context has been canceled.
				return errors.WithStack(err)
			}

			encodedData := data.Encode()
			apiURL := fmt.Sprintf("https://%s/w/api.php?%s", site, encodedData)
			req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
			if err != nil {
				errE := errors.WithStack(err)
				errors.Details(errE)["url"] = apiURL
				return errE
			}
			if token != "" {
				req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				errE := errors.WithStack(err)
				errors.Details(errE)["url"] = apiURL
				return errE
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				errE := errors.New("bad response status")
				errors.Details(errE)["url"] = apiURL
				errors.Details(errE)["code"] = resp.StatusCode
				errors.Details(errE)["body"] = strings.TrimSpace(string(body))
				return errE
			}

			var apiResp allPagesAPIResponse
			decoder := json.NewDecoder(resp.Body)
			decoder.DisallowUnknownFields()
			err = decoder.Decode(&apiResp)
			if err != nil {
				errE := errors.WithStack(err)
				errors.Details(errE)["url"] = apiURL
				return errE
			}
			if apiResp.Error != nil {
				errE := errors.New("response error")
				errors.Details(errE)["url"] = apiURL
				errors.Details(errE)["body"] = apiResp.Error
				return errE
			}

			if len(batch) == 0 {
				batch = apiResp.Query.Pages
			} else if len(batch) != len(apiResp.Query.Pages) {
				errE := errors.New("unexpected number of pages")
				errors.Details(errE)["url"] = apiURL
				errors.Details(errE)["got"] = len(apiResp.Query.Pages)
				errors.Details(errE)["expected"] = len(batch)
				return errE
			} else {
				for i, page := range apiResp.Query.Pages {
					if batch[i].Properties == nil {
						batch[i].Properties = make(map[string]string)
					}
					for key, value := range page.Properties {
						batch[i].Properties[key] = value
					}
					batch[i].Categories = append(batch[i].Categories, page.Categories...)
					batch[i].Templates = append(batch[i].Templates, page.Templates...)
					batch[i].Redirects = append(batch[i].Redirects, page.Redirects...)
				}
			}

			if apiResp.BatchComplete {
				for _, page := range batch {
					select {
					case <-ctx.Done():
						// Context has been canceled.
						return errors.WithStack(ctx.Err())
					case output <- page:
					}
				}
				batch = nil
			}

			if len(apiResp.Continue) == 0 {
				if !apiResp.BatchComplete {
					errE := errors.New("batch incomplete without continue")
					errors.Details(errE)["url"] = apiURL
					return errE
				}
				break
			}

			for key, value := range apiResp.Continue {
				data.Set(key, value)
			}
		}
	}

	return nil
}

func GetPageHTML(ctx context.Context, httpClient *retryablehttp.Client, site, title string) (string, errors.E) {
	title = strings.ReplaceAll(title, " ", "_")
	htmlURL := fmt.Sprintf("https://%s/api/rest_v1/page/html/%s", site, url.PathEscape(title))

	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, htmlURL, nil)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = htmlURL
		return "", errE
	}
	req.Header.Add("Accept-Language", "en-US")

	resp, err := httpClient.Do(req)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = htmlURL
		return "", errE
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		errE := errors.New("bad response status")
		errors.Details(errE)["url"] = htmlURL
		errors.Details(errE)["code"] = resp.StatusCode
		errors.Details(errE)["body"] = strings.TrimSpace(string(body))
		return "", errE
	}
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = htmlURL
		return "", errE
	}

	return string(body), nil
}
