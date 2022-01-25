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
	err := os.MkdirAll(config.OutputDir, 0o700)
	if err != nil {
		return errors.WithStack(err)
	}

	for id, property := range search.StandardProperties {
		path := filepath.Join(config.OutputDir, fmt.Sprintf("%s.json", id))
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
