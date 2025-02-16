package indexer

import (
	"fmt"
	"strings"
	"time"
)

const (
	// Same as go-mediawiki's progressPrintRate.
	ProgressPrintRate = 30 * time.Second
)

func StructName(v any) string {
	name := fmt.Sprintf("%T", v)
	i := strings.LastIndex(name, ".")
	return strings.ToLower(name[i+1:])
}
