package search_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/peerdb/search"
)

func TestTimestampMarshal(t *testing.T) {
	tests := []string{
		`"2006-12-04T12:34:45Z"`,
		`"0206-12-04T12:34:45Z"`,
		`"0001-12-04T12:34:45Z"`,
		`"20006-12-04T12:34:45Z"`,
		`"0000-12-04T12:34:45Z"`,
		`"-0001-12-04T12:34:45Z"`,
		`"-0206-12-04T12:34:45Z"`,
		`"-2006-12-04T12:34:45Z"`,
		`"-20006-12-04T12:34:45Z"`,
	}
	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			var time search.Timestamp
			in := []byte(test)
			err := json.Unmarshal(in, &time)
			assert.NoError(t, err)
			out, err := json.Marshal(time)
			assert.NoError(t, err)
			assert.Equal(t, in, out)
		})
	}
}
