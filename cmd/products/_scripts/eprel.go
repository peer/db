package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/x"
	z "gitlab.com/tozd/go/zerolog"

	"gitlab.com/peerdb/peerdb/internal/es"
)

type App struct {
	z.LoggingConfig `yaml:",inline"`

	Version kong.VersionFlag `help:"Show program's version and exit." short:"V" yaml:"-"`

	APIKey kong.FileContentFlag `env:"EPREL_API_KEY_PATH" help:"File with EPREL API key. Environment variable: ${env}." placeholder:"PATH" required:""`
}

func mapAllWasherDrierFields(ctx context.Context, logger zerolog.Logger, apiKey string) errors.E {
	httpClient := es.NewHTTPClient(cleanhttp.DefaultPooledClient(), logger)

	seenFields := make(map[string][]interface{})

	page := 1
	for {
		url := fmt.Sprintf("https://eprel.ec.europa.eu/api/products/washerdriers?_limit=100&_page=%d", page)
		req, err := retryablehttp.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			errE := errors.WithStack(err)
			errors.Details(errE)["url"] = url
			return errE
		}
		req.Header.Set("X-Api-Key", apiKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			errE := errors.WithStack(err)
			errors.Details(errE)["url"] = url
			return errE
		}

		var result map[string]interface{}
		errE := x.DecodeJSONWithoutUnknownFields(resp.Body, &result)
		resp.Body.Close()
		if errE != nil {
			return errE
		}

		hits, ok := result["hits"].([]interface{})
		if !ok {
			return errors.New("result['hits'] is not []interface{}")
		}
		if len(hits) == 0 {
			break
		}

		for _, hit := range hits {
			product, ok := hit.(map[string]interface{})
			if !ok {
				return errors.New("hit is not map[string]interface{}")
			}

			for field, value := range product {
				seenFields[field] = append(seenFields[field], value)
			}
		}

		page++
	}

	// Print fields and sample values.
	fields := make([]string, 0, len(seenFields))
	for field := range seenFields {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	fmt.Println("Fields and their structure:")
	for _, field := range fields {
		value := seenFields[field][0]
		fmt.Printf("- %s: %T\n", field, value)

		// Print structure of nested objects.
		if m, ok := value.(map[string]interface{}); ok {
			// TODO: Print in sorted order.
			for k := range m {
				fmt.Printf("  - %s\n", k)
			}
		} else if a, ok := value.([]interface{}); ok && len(a) > 0 {
			if m, ok := a[0].(map[string]interface{}); ok {
				// TODO: Print in sorted order.
				for k := range m {
					fmt.Printf("  - %s\n", k)
				}
			}
		}
	}

	return nil
}

func main() {
	var app App
	cli.Run(&app, nil, func(ctx *kong.Context) errors.E {
		apiKey := strings.TrimSpace(string(app.APIKey))

		return mapAllWasherDrierFields(context.Background(), app.Logger, apiKey)
	})
}
