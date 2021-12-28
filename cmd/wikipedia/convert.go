package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cosnicolaou/pbzip2"
	"gitlab.com/tozd/go/errors"
)

const (
	userAgent         = "PeerBot/1.0 (mailto:mitar.peerbot@tnode.com)"
	latestWikidataAll = "https://dumps.wikimedia.org/wikidatawiki/entities/latest-all.json.bz2"
	staleReadTimeout  = 60 * time.Second
	progressPrintRate = 30 * time.Second
)

var (
	client                  = http.DefaultClient
	bzip2DecodeThreads      int
	jsonDecodeThreads       int
	entityProcessingThreads int
)

func init() {
	bzip2DecodeThreads = runtime.GOMAXPROCS(0)
	jsonDecodeThreads = runtime.GOMAXPROCS(0)
	entityProcessingThreads = runtime.GOMAXPROCS(0)
}

func getWikidataJSONs(
	ctx context.Context, config *Config, wg *sync.WaitGroup,
	output chan<- json.RawMessage, errs chan<- errors.E,
) {
	defer wg.Done()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	matches, err := filepath.Glob(filepath.Join(config.CacheDir, "wikidata-*-all.json.bz2"))
	if err != nil {
		errs <- errors.WithStack(err)
		return
	}

	var compressedReader io.Reader
	var compressedSize int64
	var compressedRead int64
	var timer *time.Timer

	if len(matches) == 1 {
		// If we file is already cached, we use it.
		compressedFile, err := os.Open(matches[0]) //nolint:govet
		if err != nil {
			errs <- errors.WithStack(err)
			return
		}
		defer compressedFile.Close()
		compressedReader = compressedFile
		compressedSize, err = compressedFile.Seek(0, io.SeekEnd)
		if err != nil {
			errs <- errors.WithStack(err)
			return
		}
		_, err = compressedFile.Seek(0, io.SeekStart)
		if err != nil {
			errs <- errors.WithStack(err)
			return
		}
	} else if len(matches) > 1 {
		errs <- errors.Errorf("too many cached wikidata files: %d", len(matches))
		return
	} else {
		// Otherwise we download the file and cache it.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestWikidataAll, nil) //nolint:govet
		req.Header.Set("User-Agent", userAgent)
		if err != nil {
			errs <- errors.WithStack(err)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			errs <- errors.WithStack(err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errs <- errors.Errorf("bad response status (%s): %s", resp.Status, strings.TrimSpace(string(body)))
			return
		}
		compressedSizeStr := resp.Header.Get("Content-Length")
		if compressedSizeStr == "" {
			errs <- errors.Errorf("missing Content-Length header in response")
			return
		}
		compressedSize, err = strconv.ParseInt(compressedSizeStr, 10, 64) //nolint:gomnd
		if err != nil {
			errs <- errors.WithStack(err)
			return
		}
		if compressedSize == 0 {
			errs <- errors.Errorf("Content-Length header in response is zero")
			return
		}
		lastModifiedStr := resp.Header.Get("Last-Modified")
		if lastModifiedStr == "" {
			errs <- errors.Errorf("missing Last-Modified header in response")
			return
		}
		lastModified, err := http.ParseTime(lastModifiedStr)
		if err != nil {
			errs <- errors.WithStack(err)
			return
		}
		path := filepath.Join(config.CacheDir, fmt.Sprintf("wikidata-%s-all.json.bz2", lastModified.UTC().Format("20060102")))
		compressedFile, err := os.Create(path)
		if err != nil {
			errs <- errors.WithStack(err)
			return
		}
		defer func() {
			if compressedRead != compressedSize {
				// Incomplete file. Delete.
				_ = os.Remove(path)
			}
		}()
		defer compressedFile.Close()
		compressedReader = io.TeeReader(resp.Body, compressedFile)
		// TODO: Better error message when canceled.
		//       See: https://github.com/golang/go/issues/26356
		timer = time.AfterFunc(staleReadTimeout, cancel)
		defer timer.Stop()
	}

	updates := make(chan pbzip2.Progress)
	defer close(updates)

	last := time.Now()
	go func() {
		for update := range updates {
			if timer != nil {
				timer.Reset(staleReadTimeout)
			}
			compressedRead += int64(update.Compressed)
			now := time.Now()
			if now.Sub(last) >= progressPrintRate {
				last = now
				fmt.Fprintf(os.Stderr, "Progress: %0.2f%%\n", float64(compressedRead)/float64(compressedSize)*100.0) //nolint:gomnd
			}
		}
	}()

	reader := pbzip2.NewReader(
		ctx, compressedReader,
		pbzip2.DecompressionOptions(
			pbzip2.BZConcurrency(bzip2DecodeThreads),
			pbzip2.BZSendUpdates(updates),
		),
	)

	decoder := json.NewDecoder(reader)

	// Read open bracket.
	_, err = decoder.Token()
	if err != nil {
		errs <- errors.WithStack(err)
		return
	}

	for decoder.More() {
		var raw json.RawMessage
		err = decoder.Decode(&raw)
		if err != nil {
			errs <- errors.WithStack(err)
			return
		}
		if err = ctx.Err(); err != nil {
			errs <- errors.WithStack(err)
			return
		}
		output <- raw
	}

	// Read closing bracket.
	_, err = decoder.Token()
	if err != nil {
		errs <- errors.WithStack(err)
		return
	}
}

func unmarshalWithoutUnknownFields(data []byte, v interface{}) errors.E {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(v)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func decodeJSONs(
	ctx context.Context, config *Config, wg *sync.WaitGroup,
	input <-chan json.RawMessage, output chan<- Entity, errs chan<- errors.E,
) {
	defer wg.Done()

	for {
		select {
		case raw, ok := <-input:
			if !ok {
				return
			}
			var e Entity
			err := unmarshalWithoutUnknownFields(raw, &e)
			if err != nil {
				errs <- errors.Wrapf(err, "cannot decode json: %s", raw)
				return
			}
			output <- e
		case <-ctx.Done():
			errs <- errors.WithStack(ctx.Err())
			return
		}
	}
}

func processEntities(
	ctx context.Context, config *Config, wg *sync.WaitGroup,
	input <-chan Entity, errs chan<- errors.E,
) {
	defer wg.Done()

	for {
		select {
		case entity, ok := <-input:
			if !ok {
				return
			}
			err := processEntity(entity)
			if err != nil {
				errs <- err
				return
			}
		case <-ctx.Done():
			errs <- errors.WithStack(ctx.Err())
			return
		}
	}
}

func convert(config *Config) errors.E {
	// We call cancel on SIGINT or SIGTERM signal and on any
	// error from goroutines. The expectation is that all
	// goroutines return soon afterwards.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// mainWg counts groups of same goroutines.
	var mainWg sync.WaitGroup
	// mainWgChan is closed when mainWg is done.
	mainWgChan := make(chan struct{})

	// Call cancel on SIGINT or SIGTERM signal.
	go func() {
		c := make(chan os.Signal, 1)
		defer close(c)

		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(c)

		// We wait for a signal or that the context is canceled
		// or that all goroutines are done.
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		case <-mainWgChan:
		}
	}()

	errs := make(chan errors.E, 1+jsonDecodeThreads+entityProcessingThreads)
	defer close(errs)

	jsons := make(chan json.RawMessage, jsonDecodeThreads)
	entities := make(chan Entity, entityProcessingThreads)

	var getWikidataJSONsWg sync.WaitGroup
	mainWg.Add(1)
	getWikidataJSONsWg.Add(1)
	go getWikidataJSONs(ctx, config, &getWikidataJSONsWg, jsons, errs)
	go func() {
		getWikidataJSONsWg.Wait()
		mainWg.Done()
		// All goroutines using jsons channel as output are done,
		// we can close the channel.
		close(jsons)
	}()

	var decodeJSONsWg sync.WaitGroup
	mainWg.Add(1)
	for w := 0; w < jsonDecodeThreads; w++ {
		decodeJSONsWg.Add(1)
		go decodeJSONs(ctx, config, &decodeJSONsWg, jsons, entities, errs)
	}
	go func() {
		decodeJSONsWg.Wait()
		mainWg.Done()
		// All goroutines using entities channel as output are done,
		// we can close the channel.
		close(entities)
	}()

	var processEntityWg sync.WaitGroup
	mainWg.Add(1)
	for w := 0; w < entityProcessingThreads; w++ {
		processEntityWg.Add(1)
		go processEntities(ctx, config, &processEntityWg, entities, errs)
	}
	go func() {
		processEntityWg.Wait()
		mainWg.Done()
	}()

	// When mainWg is done, we close mainWgChan.
	// This means that all goroutines are done.
	go func() {
		mainWg.Wait()
		close(mainWgChan)
	}()

	allErrors := []errors.E{}
WAIT:
	for {
		// We cancel the context on any error, but we also store it.
		// We also wait for all goroutines to return. The expectation
		// is that they return all when they are all successful, or
		// when there was an error and we cancelled the context.
		select {
		case err := <-errs:
			allErrors = append(allErrors, err)
			cancel()
		case <-mainWgChan:
			break WAIT
		}
	}

	if len(allErrors) > 0 {
		// If there is any non-context-canceled error, return it.
		// TODO: What if there are multiple such errors?
		for _, err := range allErrors {
			if !errors.Is(err, context.Canceled) {
				return err
			}
		}

		// Otherwise return any error, i.e., the first.
		return allErrors[0]
	}

	return nil
}
