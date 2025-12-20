package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/cli"
	"gitlab.com/tozd/go/errors"
	z "gitlab.com/tozd/go/zerolog"

	"gitlab.com/peerdb/peerdb/internal/eprel"
	"gitlab.com/peerdb/peerdb/internal/es"
)

type App struct {
	z.LoggingConfig `yaml:",inline"`

	Version kong.VersionFlag `help:"Show program's version and exit." short:"V" yaml:"-"`

	APIKey kong.FileContentFlag `env:"EPREL_API_KEY_PATH" help:"File with EPREL API key. Environment variable: ${env}." placeholder:"PATH" required:""`
}

func mapAllWasherDrierFields(ctx context.Context, logger zerolog.Logger, apiKey string) errors.E {
	httpClient := es.NewHTTPClient(cleanhttp.DefaultPooledClient(), logger)

	washerDriers, errE := eprel.GetWasherDriers[map[string]any](ctx, httpClient, apiKey)
	if errE != nil {
		return errE
	}

	seenFields := make(map[string][]any)
	for _, washerDrier := range washerDriers {
		for field, value := range washerDrier {
			seenFields[field] = append(seenFields[field], value)
		}
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
