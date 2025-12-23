package eprel

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

func GetWasherDriers[T any](ctx context.Context, httpClient *retryablehttp.Client, apiKey string) ([]T, errors.E) {
	type washerDrierResponse struct {
		Offset int `json:"offset"`
		Size   int `json:"size"`
		Hits   []T `json:"hits"`
	}

	var allWasherDriers []T
	limit := 100
	page := 1

	var totalSize int

	for {
		baseURL := "https://eprel.ec.europa.eu/api/products/washerdriers"
		params := url.Values{}
		params.Add("_limit", strconv.Itoa(limit))
		params.Add("_page", strconv.Itoa(page))

		url := fmt.Sprintf("%s?%s", baseURL, params.Encode())

		req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			errE := errors.WithStack(err)
			errors.Details(errE)["url"] = url
			return nil, errE
		}

		req.Header.Set("X-Api-Key", apiKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			errE := errors.WithStack(err)
			errors.Details(errE)["url"] = url
			return nil, errE
		}

		var result washerDrierResponse
		errE := x.DecodeJSONWithoutUnknownFields(resp.Body, &result)
		resp.Body.Close() //nolint:errcheck
		if errE != nil {
			return nil, errE
		}

		if len(result.Hits) == 0 {
			break
		}

		if page == 1 {
			totalSize = result.Size
		}

		allWasherDriers = append(allWasherDriers, result.Hits...)
		page++
	}

	if len(allWasherDriers) != totalSize {
		errE := errors.New("unexpected number of washer driers")
		errors.Details(errE)["expected"] = totalSize
		errors.Details(errE)["got"] = len(allWasherDriers)
		return nil, errE
	}

	return allWasherDriers, nil
}
