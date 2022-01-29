package search

import (
	"fmt"
	"net/http"
	"os"

	"gitlab.com/tozd/go/errors"
)

var NotFound = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
	http.NotFound(w, req)
})

func Error(w http.ResponseWriter, req *http.Request, err errors.E) {
	// TODO: Use logger.
	fmt.Fprintf(os.Stderr, "internal server error: %+v", err)
	http.Error(w, "500 internal server error", http.StatusInternalServerError)
}
