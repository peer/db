package main

import (
	"bufio"
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata
var content embed.FS

func TestProcessFursDejFile(t *testing.T) {
	t.Parallel()

	file, err := content.Open("testdata/DURS_zavezanci_DEJ.txt")
	if err != nil {
		require.NoError(t, err)
	}
	t.Cleanup(func() { file.Close() }) //nolint:errcheck

	readFile := bufio.NewReader(file)

	records, errE := processFursDejFile(readFile)
	if errE != nil {
		require.NoError(t, errE, "% -+#.1v", errE)
	}
	require.Len(t, records, 8, "expected 8 records, but got %d", len(records))

	assert.NotEmpty(t, records)

	// Check the problematic record, if SKD is an empty string.
	assert.Empty(t, records[5].SKD, "SKD mismatch")

	for record := range records {
		assert.Len(t, records[record].VATNumber, 8, "VATNumber should be 8 characters long")
		assert.Len(t, records[record].RegistrationNumber, 10, "RegistrationNumber should be 10 characters long")
		assert.Len(t, records[record].FinancialOffice, 2, "FinancialOffice should be 2 characters long")
		assert.NotEmpty(t, records[record].Name, "Name should not be empty")
		assert.NotEmpty(t, records[record].Address, "Address should not be empty")
		if record == 5 || record == 6 {
			continue
		}
		assert.Len(t, records[record].SKD, 6, "SKD should be 'XX.XXX' 6 characters long")
	}
}
