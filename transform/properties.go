package transform

import (
	"context"
	"reflect"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// Mnemonics returns a map between property mnemonic and identifier.
//
// It takes a slice of any structs and extracts the mnemonic (from a field named `Mnemonic`)
// to identifier (from a field named `ID`) mapping from them.
//
// Returns an error if mnemonics are not unique.
func Mnemonics(ctx context.Context, documents []any) (map[string]identifier.Identifier, errors.E) {
	result := map[string]identifier.Identifier{}

	for _, doc := range documents {
		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

		mnemonicValue, errE := extractFieldValue(doc, "Mnemonic")
		if errE != nil {
			return nil, errE
		} else if !mnemonicValue.IsValid() {
			continue
		}

		if mnemonicValue.Kind() != reflect.String {
			errE := errors.Errorf("expected string for mnemonic")
			errors.Details(errE)["type"] = mnemonicValue.Type().String()
			return nil, errE
		}
		mnemonic := mnemonicValue.String()

		if mnemonic == "" {
			continue
		}

		idValue, errE := extractFieldValue(doc, "ID")
		if errE != nil {
			return nil, errE
		} else if !idValue.IsValid() {
			continue
		}

		id, ok := idValue.Interface().([]string)
		if !ok {
			errE := errors.Errorf("expected []string for ID")
			errors.Details(errE)["type"] = idValue.Type().String()
			return nil, errE
		}

		if len(id) == 0 {
			continue
		}

		if _, ok := result[mnemonic]; ok {
			errE := errors.Errorf("duplicate mnemonic")
			errors.Details(errE)["mnemonic"] = mnemonic
			return nil, errE
		}

		result[mnemonic] = identifier.From(id...)
	}

	return result, nil
}

func extractFieldValue(doc any, fieldName string) (reflect.Value, errors.E) {
	v := reflect.ValueOf(doc)
	// Handle pointer to struct.
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return reflect.Value{}, nil
	}

	return extractFieldValueFromStruct(v, fieldName)
}

func extractFieldValueFromStruct(structValue reflect.Value, fieldName string) (reflect.Value, errors.E) {
	structType := structValue.Type()

	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)

		if field.Name == fieldName {
			return fieldValue, nil
		}

		if field.Anonymous && fieldValue.Kind() == reflect.Struct {
			v, errE := extractFieldValueFromStruct(fieldValue, fieldName)
			if errE != nil {
				return reflect.Value{}, errE
			} else if v.IsValid() {
				return v, nil
			}
		}
	}

	return reflect.Value{}, nil
}
