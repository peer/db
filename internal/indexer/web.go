package indexer

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/foolin/pagser"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/temoto/robotstxt"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
)

type SchemaOrg struct {
	Type        string `json:"@type"`
	Name        string `json:"name"`
	Image       string `json:"image"`
	ID          string `json:"identifier"`
	SKU         string `json:"sku"`
	Brand       string `json:"brand"`
	Category    string `json:"category"`
	Description string `json:"description"`
	URL         string `json:"url"`
	MPN         string `json:"mpn"`
}

func pagserExists(node *goquery.Selection, _ ...string) (interface{}, error) {
	return node.Length() > 0, nil
}

func pagserClass(node *goquery.Selection, args ...string) (interface{}, error) {
	classPrefix := ""
	if len(args) > 0 {
		classPrefix = args[0]
	}

	list := strings.Split(node.AttrOr("class", ""), " ")
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v != "" && strings.HasPrefix(v, classPrefix) {
			return strings.TrimPrefix(v, classPrefix), nil
		}
	}

	return "", nil
}

func pagserSchemaOrg(node *goquery.Selection, _ ...string) (interface{}, error) {
	var s SchemaOrg
	errE := x.Unmarshal([]byte(node.Text()), &s)
	if errE != nil {
		return nil, errE
	}
	return s, nil
}

func ExtractData[T any](in io.Reader) (T, errors.E) { //nolint:ireturn
	config := pagser.DefaultConfig()
	config.CastError = true
	p, _ := pagser.NewWithConfig(config)

	p.RegisterFunc("exists", pagserExists)
	p.RegisterFunc("schemaOrg", pagserSchemaOrg)
	p.RegisterFunc("class", pagserClass)

	var data T
	err := p.ParseReader(&data, in)
	if err != nil {
		return *new(T), errors.WithStack(err)
	}

	return data, nil
}

// TODO: Respect robots.txt.
// TODO: Make sure we are making only one request per domain at once.

func GetWebData[T any](ctx context.Context, httpClient *retryablehttp.Client, url string, f func(in io.Reader) (T, errors.E)) (T, errors.E) { //nolint:ireturn
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
	defer resp.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errE := errors.New("bad response status")
		errors.Details(errE)["url"] = url
		errors.Details(errE)["code"] = resp.StatusCode
		errors.Details(errE)["body"] = strings.TrimSpace(string(body))
		return *new(T), errE
	}

	return f(resp.Body)
}

// TODO: Cache robots.txt per domain.

func GetRobotsTxt(ctx context.Context, httpClient *retryablehttp.Client, u string) (*robotstxt.RobotsData, errors.E) {
	url, err := url.Parse(u)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = u
		return nil, errE
	}
	url.Path = "/robots.txt"
	url.RawPath = ""
	url.RawQuery = ""
	url.ForceQuery = false
	url.Fragment = ""
	url.RawFragment = ""
	u = url.String()

	req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = u
		return nil, errE
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = u
		return nil, errE
	}
	defer resp.Body.Close()              //nolint:errcheck
	defer io.Copy(io.Discard, resp.Body) //nolint:errcheck

	robots, err := robotstxt.FromResponse(resp)
	if err != nil {
		errE := errors.WithStack(err)
		errors.Details(errE)["url"] = u
		return nil, errE
	}

	return robots, nil
}
