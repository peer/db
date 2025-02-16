package indexer

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/foolin/pagser"
	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
)

func pagserExists(node *goquery.Selection, _ ...string) (interface{}, error) {
	return node.Length() > 0, nil
}

func ExtractData[T any](in io.Reader) (T, errors.E) { //nolint:ireturn
	p := pagser.New()

	p.RegisterFunc("exists", pagserExists)

	var data T
	err := p.ParseReader(&data, in)
	if err != nil {
		return *new(T), errors.WithStack(err)
	}

	return data, nil
}

func GetWebData[T any](ctx context.Context, httpClient *retryablehttp.Client, url string) (T, errors.E) { //nolint:ireturn
	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = url
		return *new(T), errE
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = url
		return *new(T), errE
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errE := errors.New("bad response status")
		errors.Details(errE)["url"] = url
		errors.Details(errE)["code"] = resp.StatusCode
		errors.Details(errE)["body"] = strings.TrimSpace(string(body))
		return *new(T), errE
	}

	return ExtractData[T](resp.Body)
}
