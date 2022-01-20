package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/search"
)

func saveStandardProperties(config *Config) errors.E {
	outputDir := filepath.Join(config.OutputDir, "properties")

	err := os.MkdirAll(outputDir, 0o700)
	if err != nil {
		return errors.WithStack(err)
	}

	for id, property := range search.KnownProperties {
		path := filepath.Join(outputDir, fmt.Sprintf("%s.json", id))
		file, err := os.Create(path)
		if err != nil {
			return errors.WithStack(err)
		}
		defer file.Close()
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		err = encoder.Encode(property)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
