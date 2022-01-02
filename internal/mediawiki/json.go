package mediawiki

import (
	"bytes"
	"encoding/json"

	"gitlab.com/tozd/go/errors"
)

func UnmarshalWithoutUnknownFields(data []byte, v interface{}) errors.E {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(v)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
