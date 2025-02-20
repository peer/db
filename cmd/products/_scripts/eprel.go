package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
)

func getAPIKey() (string, error) {
	key, err := os.ReadFile("../../.eprel.secret")
	if err != nil {
		return "", errors.WithStack(err)
	}
	return strings.TrimSpace(string(key)), nil
}

func mapAllWasherDrierFields(ctx context.Context, apiKey string) error {
	httpClient := retryablehttp.NewClient()
	httpClient.Logger = nil

	seenFields := make(map[string][]interface{})

	page := 1
	for {
		url := fmt.Sprintf("https://eprel.ec.europa.eu/api/products/washerdriers?_limit=100&_page=%d", page)
		req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return errors.WithStack(err)
		}
		req.Header.Set("X-Api-Key", apiKey)

		resp, err := httpClient.Do(req)
		if err != nil {
			return errors.WithStack(err)
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return errors.WithStack(err)
		}
		resp.Body.Close()

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
				if _, exists := seenFields[field]; !exists {
					seenFields[field] = []interface{}{value}
				}
			}
		}

		page++
	}

	// Print fields and sample values
	fields := make([]string, 0, len(seenFields))
	for field := range seenFields {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	fmt.Println("Fields and their structure:")
	for _, field := range fields {
		value := seenFields[field][0]
		fmt.Printf("- %s: %T\n", field, value)

		// Print structure of nested objects
		if m, ok := value.(map[string]interface{}); ok {
			for k := range m {
				fmt.Printf("  - %s\n", k)
			}
		} else if a, ok := value.([]interface{}); ok && len(a) > 0 {
			if m, ok := a[0].(map[string]interface{}); ok {
				for k := range m {
					fmt.Printf("  - %s\n", k)
				}
			}
		}
	}

	return nil
}

func main() {
	apiKey, err := getAPIKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading API key: %v\n", err)
		os.Exit(1)
	}

	if err := mapAllWasherDrierFields(context.Background(), apiKey); err != nil {
		fmt.Fprintf(os.Stderr, "Error mapping fields: %v\n", err)
		os.Exit(1)
	}
}
