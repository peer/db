package peerdb_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/rs/zerolog"
	z "gitlab.com/tozd/go/zerolog"

	"gitlab.com/peerdb/peerdb"
)

func TestMain(m *testing.M) {
	elastic := os.Getenv("ELASTIC")
	if elastic == "" {
		elastic = peerdb.DefaultElastic
	}

	globals := &peerdb.Globals{ //nolint:exhaustruct
		LoggingConfig: z.LoggingConfig{ //nolint:exhaustruct
			Logger: zerolog.Nop(),
		},
		Elastic:   elastic,
		Index:     peerdb.DefaultIndex,
		SizeField: false,
	}

	populate := peerdb.PopulateCommand{}

	errE := populate.Run(globals)
	if errE != nil {
		fmt.Fprintf(os.Stderr, "% -+#.1v\n", errE)
		os.Exit(1)
	}

	m.Run()
}
