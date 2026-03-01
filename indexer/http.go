package indexer

import (
	"net/http"
	"net/url"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
)

func prepareFields(keysAndValues []interface{}) {
	for i, keyOrValue := range keysAndValues {
		// We want URLs logged as strings.
		u, ok := keyOrValue.(*url.URL)
		if ok {
			keysAndValues[i] = u.String()
		}
	}
}

type retryableHTTPLoggerAdapter struct {
	logger zerolog.Logger
}

func (a retryableHTTPLoggerAdapter) Error(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.logger.Error().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Info(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.logger.Info().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Debug(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.logger.Debug().Fields(keysAndValues).Msg(msg)
}

func (a retryableHTTPLoggerAdapter) Warn(msg string, keysAndValues ...interface{}) {
	prepareFields(keysAndValues)
	a.logger.Warn().Fields(keysAndValues).Msg(msg)
}

var _ retryablehttp.LeveledLogger = (*retryableHTTPLoggerAdapter)(nil)

// NewHTTPClient creates a retryable HTTP client with the specified logger, optional base HTTP client, and user agent header.
func NewHTTPClient(logger zerolog.Logger, httpClient *http.Client, userAgent string) *http.Client {
	if httpClient == nil {
		httpClient = cleanhttp.DefaultPooledClient()
	}

	client := retryablehttp.NewClient()
	client.HTTPClient = httpClient
	client.RetryWaitMax = clientRetryWaitMax
	client.RetryMax = clientRetryMax
	client.Logger = retryableHTTPLoggerAdapter{logger}

	// Set User-Agent header.
	client.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, _ int) {
		req.Header.Set("User-Agent", userAgent)
	}

	return client.StandardClient()
}
