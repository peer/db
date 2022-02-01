package search

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"gitlab.com/tozd/go/errors"
)

var NotFound = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
	http.NotFound(w, req)
})

func InternalError(w http.ResponseWriter, req *http.Request, err errors.E) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}

	// TODO: Use logger.
	fmt.Fprintf(os.Stderr, "internal server error: %+v", err)
	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}

func BadRequestError(w http.ResponseWriter, req *http.Request, err errors.E) {
	// TODO: Use logger.
	fmt.Fprintf(os.Stderr, "bad request: %+v", err)
	http.Error(w, "400 bad request", http.StatusBadRequest)
}
