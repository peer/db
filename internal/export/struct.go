package export

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/core"
	"gitlab.com/peerdb/peerdb/document"
	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// baseCache caches document Base ([]string) to avoid repeated fetches.
type baseCache struct {
	bases  map[identifier.Identifier][]string
	getDoc GetDocFunc
}

// newBaseCache creates a new baseCache.
func newBaseCache(getDoc GetDocFunc) *baseCache {
	return &baseCache{
		bases:  make(map[identifier.Identifier][]string),
		getDoc: getDoc,
	}
}

// getBase returns the Base ([]string) for a document ID, fetching and caching as needed.
func (c *baseCache) getBase(ctx context.Context, id identifier.Identifier) ([]string, errors.E) {
	if base, ok := c.bases[id]; ok {
		return base, nil
	}
	doc, errE := c.getDoc(ctx, id)
	if errE != nil {
		return nil, errE
	}
	if doc == nil {
		return nil, nil
	}
	c.bases[id] = doc.Base
	return doc.Base, nil
}

// Struct exports documents as JSON-per-line, using the registered Go struct types
// from core.ClassRegistry to reconstruct the struct from claims.
func Struct(ctx context.Context, w io.Writer, docIDs []identifier.Identifier,
	mnemonics map[string]identifier.Identifier, getDoc GetDocFunc,
) errors.E {
	logger := zerolog.Ctx(ctx)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	cache := newBaseCache(getDoc)

	// Build reverse map: property ID -> mnemonic.
	propToMnemonic := make(map[identifier.Identifier]string, len(mnemonics))
	for mnemonic, id := range mnemonics {
		propToMnemonic[id] = mnemonic
	}

	for _, docID := range docIDs {
		doc, errE := getDoc(ctx, docID)
		if errE != nil {
			return errE
		}
		if doc == nil {
			continue
		}

		// Find INSTANCE_OF class ID.
		classIDs := findInstanceOfClassIDs(doc)
		if len(classIDs) == 0 {
			logger.Warn().Str("docID", docID.String()).Msg("document has no INSTANCE_OF claim, skipping")
			continue
		}

		// Look up registered type for the first matching class.
		var typ reflect.Type
		var matchedClassID identifier.Identifier
		for _, cid := range classIDs {
			if t, ok := core.ClassRegistry[cid]; ok {
				typ = t
				matchedClassID = cid
				break
			}
		}
		if typ == nil {
			logger.Warn().Str("docID", docID.String()).Msg("no registered struct type for document class, skipping")
			continue
		}

		// Create new struct instance.
		structVal := reflect.New(typ).Elem()

		// Set DocumentFields.ID from doc.Base.
		setDocumentFieldsID(structVal, typ, doc.Base)

		// Set DocumentFields.InstanceOf from INSTANCE_OF reference claims.
		errE = setDocumentFieldsInstanceOf(ctx, structVal, typ, doc, cache)
		if errE != nil {
			return errE
		}

		// Collect matched property IDs to detect unmatched claims.
		matchedPropIDs := map[identifier.Identifier]bool{
			internalCore.InstanceOfPropID: true,
		}

		// Walk struct fields with property tags and populate from claims.
		errE = populateStructFromClaims(ctx, logger, structVal, typ, doc, mnemonics, matchedPropIDs, cache, matchedClassID)
		if errE != nil {
			return errE
		}

		// Log unmatched claims.
		logUnmatchedClaims(ctx, logger, doc, matchedPropIDs, propToMnemonic)

		// Encode struct as JSON.
		err := enc.Encode(structVal.Addr().Interface())
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// findInstanceOfClassIDs extracts INSTANCE_OF reference claim target IDs from a document.
func findInstanceOfClassIDs(doc *document.D) []identifier.Identifier {
	claims := doc.Get(internalCore.InstanceOfPropID)
	result := make([]identifier.Identifier, 0, len(claims))
	for _, c := range claims {
		if ref, ok := c.(*document.ReferenceClaim); ok {
			result = append(result, ref.To.ID)
		}
	}
	return result
}

// setDocumentFieldsID sets the ID field ([]string with documentid tag) on a struct.
func setDocumentFieldsID(structVal reflect.Value, structType reflect.Type, base []string) {
	setDocumentFieldsIDRecursive(structVal, structType, base)
}

// setDocumentFieldsIDRecursive recursively searches for the documentid-tagged field.
func setDocumentFieldsIDRecursive(structVal reflect.Value, structType reflect.Type, base []string) bool {
	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldVal := structVal.Field(i)

		if _, ok := field.Tag.Lookup("documentid"); ok {
			if fieldVal.Kind() == reflect.Slice && fieldVal.Type().Elem().Kind() == reflect.String {
				fieldVal.Set(reflect.ValueOf(slices.Clone(base)))
				return true
			}
		}

		if field.Anonymous && fieldVal.Kind() == reflect.Struct {
			if setDocumentFieldsIDRecursive(fieldVal, fieldVal.Type(), base) {
				return true
			}
		}
	}
	return false
}

// setDocumentFieldsInstanceOf sets the InstanceOf field from INSTANCE_OF reference claims.
func setDocumentFieldsInstanceOf(
	ctx context.Context, structVal reflect.Value, structType reflect.Type,
	doc *document.D, cache *baseCache,
) errors.E {
	return setInstanceOfRecursive(ctx, structVal, structType, doc, cache)
}

// setInstanceOfRecursive recursively searches for the INSTANCE_OF property-tagged field.
func setInstanceOfRecursive(
	ctx context.Context, structVal reflect.Value, structType reflect.Type,
	doc *document.D, cache *baseCache,
) errors.E {
	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldVal := structVal.Field(i)

		if field.Tag.Get("property") == "INSTANCE_OF" {
			// Build Ref slice from INSTANCE_OF reference claims.
			claims := doc.Get(internalCore.InstanceOfPropID)
			refs := make([]internalCore.Ref, 0, len(claims))
			for _, c := range claims {
				if refClaim, ok := c.(*document.ReferenceClaim); ok {
					base, errE := cache.getBase(ctx, refClaim.To.ID)
					if errE != nil {
						return errE
					}
					if base != nil {
						refs = append(refs, internalCore.Ref{ID: slices.Clone(base)})
					}
				}
			}
			if len(refs) > 0 {
				fieldVal.Set(reflect.ValueOf(refs))
			}
			return nil
		}

		if field.Anonymous && fieldVal.Kind() == reflect.Struct {
			errE := setInstanceOfRecursive(ctx, fieldVal, fieldVal.Type(), doc, cache)
			if errE != nil {
				return errE
			}
		}
	}
	return nil
}

// populateStructFromClaims walks struct fields with property tags and populates them from document claims.
func populateStructFromClaims(
	ctx context.Context, logger *zerolog.Logger,
	structVal reflect.Value, structType reflect.Type,
	doc *document.D,
	mnemonics map[string]identifier.Identifier,
	matchedPropIDs map[identifier.Identifier]bool,
	cache *baseCache,
	classID identifier.Identifier,
) errors.E {
	return populateFieldsRecursive(ctx, logger, structVal, structType, doc, mnemonics, matchedPropIDs, cache, classID)
}

// populateFieldsRecursive recursively processes struct fields.
func populateFieldsRecursive(
	ctx context.Context, logger *zerolog.Logger,
	structVal reflect.Value, structType reflect.Type,
	claimsContainer document.Claims,
	mnemonics map[string]identifier.Identifier,
	matchedPropIDs map[identifier.Identifier]bool,
	cache *baseCache,
	classID identifier.Identifier,
) errors.E {
	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldVal := structVal.Field(i)

		// Skip documentid fields.
		if _, ok := field.Tag.Lookup("documentid"); ok {
			continue
		}

		// Skip value fields.
		if _, ok := field.Tag.Lookup("value"); ok {
			continue
		}

		mnemonic := field.Tag.Get("property")
		if mnemonic == "-" {
			continue
		}

		// Handle embedded structs.
		if field.Anonymous && fieldVal.Kind() == reflect.Struct {
			errE := populateFieldsRecursive(ctx, logger, fieldVal, fieldVal.Type(), claimsContainer, mnemonics, matchedPropIDs, cache, classID)
			if errE != nil {
				return errE
			}
			continue
		}

		if mnemonic == "" {
			continue
		}

		// Skip INSTANCE_OF, handled separately.
		if mnemonic == "INSTANCE_OF" {
			continue
		}

		// Resolve property ID from mnemonic.
		propID, ok := mnemonics[mnemonic]
		if !ok {
			logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Msg("mnemonic not found in mnemonics map, skipping field")
			continue
		}

		matchedPropIDs[propID] = true

		// Get matching claims.
		claims := claimsContainer.Get(propID)
		if len(claims) == 0 {
			continue
		}

		// Set field value based on cardinality.
		errE := setFieldFromClaims(ctx, logger, fieldVal, field.Type, claims, mnemonics, cache, classID, mnemonic)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// setFieldFromClaims sets a struct field's value from matching claims.
func setFieldFromClaims(
	ctx context.Context, logger *zerolog.Logger,
	fieldVal reflect.Value, fieldType reflect.Type,
	claims []document.Claim,
	mnemonics map[string]identifier.Identifier,
	cache *baseCache,
	classID identifier.Identifier,
	mnemonic string,
) errors.E {
	switch fieldVal.Kind() { //nolint:exhaustive
	case reflect.Slice:
		// Collect all matching claim values.
		return setSliceFieldFromClaims(ctx, logger, fieldVal, fieldType, claims, mnemonics, cache, classID, mnemonic)
	case reflect.Pointer:
		// Take first matching claim.
		if len(claims) > 1 {
			logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Int("count", len(claims)).Msg("excess claims for pointer field, using first")
		}
		return setPtrFieldFromClaim(ctx, logger, fieldVal, fieldType, claims[0], mnemonics, cache, classID, mnemonic)
	default:
		// Single value: take first claim.
		if len(claims) > 1 {
			logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Int("count", len(claims)).Msg("excess claims for single-value field, using first")
		}
		return setSingleFieldFromClaim(ctx, logger, fieldVal, fieldType, claims[0], mnemonics, cache, classID, mnemonic)
	}
}

// setSliceFieldFromClaims populates a slice field from all matching claims.
func setSliceFieldFromClaims(
	ctx context.Context, logger *zerolog.Logger,
	fieldVal reflect.Value, fieldType reflect.Type,
	claims []document.Claim,
	mnemonics map[string]identifier.Identifier,
	cache *baseCache,
	classID identifier.Identifier,
	mnemonic string,
) errors.E {
	elemType := fieldType.Elem()
	result := reflect.MakeSlice(fieldType, 0, len(claims))

	for _, claim := range claims {
		elemVal := reflect.New(elemType).Elem()
		errE := setSingleFieldFromClaim(ctx, logger, elemVal, elemType, claim, mnemonics, cache, classID, mnemonic)
		if errE != nil {
			return errE
		}
		result = reflect.Append(result, elemVal)
	}

	fieldVal.Set(result)
	return nil
}

// setPtrFieldFromClaim populates a pointer field from a single claim.
func setPtrFieldFromClaim(
	ctx context.Context, logger *zerolog.Logger,
	fieldVal reflect.Value, fieldType reflect.Type,
	claim document.Claim,
	mnemonics map[string]identifier.Identifier,
	cache *baseCache,
	classID identifier.Identifier,
	mnemonic string,
) errors.E {
	elemType := fieldType.Elem()
	elemVal := reflect.New(elemType)
	errE := setSingleFieldFromClaim(ctx, logger, elemVal.Elem(), elemType, claim, mnemonics, cache, classID, mnemonic)
	if errE != nil {
		return errE
	}
	fieldVal.Set(elemVal)
	return nil
}

// setSingleFieldFromClaim sets a single (non-slice, non-pointer) field from a claim.
//

func setSingleFieldFromClaim(
	ctx context.Context, logger *zerolog.Logger,
	fieldVal reflect.Value, fieldType reflect.Type,
	claim document.Claim,
	mnemonics map[string]identifier.Identifier,
	cache *baseCache,
	classID identifier.Identifier,
	mnemonic string,
) errors.E {
	// Check if field is a struct with value tag (nested struct with sub-claims).
	if fieldType.Kind() == reflect.Struct {
		// Check if field type is a known core type first.
		if setKnownCoreType(ctx, logger, fieldVal, fieldType, claim, cache, classID, mnemonic) {
			// Sub-claims for known core types are handled within setKnownCoreType.
			return nil
		}

		// Otherwise handle as nested struct.
		return setNestedStructFromClaim(ctx, logger, fieldVal, fieldType, claim, mnemonics, cache, classID, mnemonic)
	}

	// Handle by claim type.
	switch c := claim.(type) {
	case *document.StringClaim:
		return setStringLikeValue(fieldVal, fieldType, c.String, classID, mnemonic, logger)
	case *document.IdentifierClaim:
		return setStringLikeValue(fieldVal, fieldType, c.Value, classID, mnemonic, logger)
	case *document.LinkClaim:
		return setStringLikeValue(fieldVal, fieldType, c.IRI, classID, mnemonic, logger)
	case *document.HTMLClaim:
		return setStringLikeValue(fieldVal, fieldType, c.HTML, classID, mnemonic, logger)
	case *document.AmountClaim:
		return setAmountValue(fieldVal, fieldType, c, classID, mnemonic, logger)
	case *document.ReferenceClaim:
		return setReferenceValue(ctx, fieldVal, fieldType, c, cache, classID, mnemonic, logger)
	case *document.TimeClaim:
		return setTimeValue(fieldVal, fieldType, c, classID, mnemonic, logger)
	case *document.NoneClaim:
		return setBoolValue(fieldVal, fieldType, true, classID, mnemonic, logger)
	case *document.UnknownClaim:
		return setBoolValue(fieldVal, fieldType, true, classID, mnemonic, logger)
	case *document.HasClaim:
		// HasClaim with bool field: set to true.
		if fieldType.Kind() == reflect.Bool {
			fieldVal.SetBool(true)
			return nil
		}
		logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).
			Str("claimType", "HasClaim").Str("fieldType", fieldType.String()).
			Msg("type mismatch for field")
		return nil
	default:
		logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Str("claimType", fmt.Sprintf("%T", claim)).Msg("unsupported claim type")
		return nil
	}
}

// setKnownCoreType handles setting known core struct types (Ref, Time, Amount, Interval).
// Returns true if the type was recognized and handled.
func setKnownCoreType(
	ctx context.Context, logger *zerolog.Logger,
	fieldVal reflect.Value, fieldType reflect.Type,
	claim document.Claim,
	cache *baseCache,
	classID identifier.Identifier,
	mnemonic string,
) bool {
	switch fieldType {
	case internalCore.RefType:
		if refClaim, ok := claim.(*document.ReferenceClaim); ok {
			base, errE := cache.getBase(ctx, refClaim.To.ID)
			if errE != nil {
				logger.Warn().Err(errE).Str("mnemonic", mnemonic).Msg("failed to fetch reference base")
				return true
			}
			if base != nil {
				fieldVal.Set(reflect.ValueOf(internalCore.Ref{ID: slices.Clone(base)}))
			}
		} else {
			logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Str("claimType", fmt.Sprintf("%T", claim)).Msg("expected ReferenceClaim for Ref field")
		}
		return true

	case internalCore.TimeType:
		if timeClaim, ok := claim.(*document.TimeClaim); ok {
			fieldVal.Set(reflect.ValueOf(internalCore.Time{
				Time:      mustParseDocTime(timeClaim.Time, timeClaim.Precision),
				Precision: timeClaim.Precision,
			}))
		} else {
			logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Str("claimType", fmt.Sprintf("%T", claim)).Msg("expected TimeClaim for Time field")
		}
		return true

	default:
		// Check if it is an Amount type.
		if setAmountStructValue(fieldVal, fieldType, claim, classID, mnemonic, logger) {
			return true
		}
		// Check if it is an Interval type.
		if setIntervalValue(fieldVal, fieldType, claim, classID, mnemonic, logger) {
			return true
		}
		return false
	}
}

// setNestedStructFromClaim handles setting a nested struct (with value and property tags) from a claim.
func setNestedStructFromClaim(
	ctx context.Context, logger *zerolog.Logger,
	fieldVal reflect.Value, fieldType reflect.Type,
	claim document.Claim,
	mnemonics map[string]identifier.Identifier,
	cache *baseCache,
	classID identifier.Identifier,
	mnemonic string,
) errors.E {
	// Find and set the value field if present.
	for i := range fieldType.NumField() {
		sf := fieldType.Field(i)
		if _, ok := sf.Tag.Lookup("value"); ok {
			vf := fieldVal.Field(i)
			errE := setSingleFieldFromClaim(ctx, logger, vf, sf.Type, claim, mnemonics, cache, classID, mnemonic)
			if errE != nil {
				return errE
			}
			break
		}
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
			// Check embedded structs for value field.
			if findAndSetValueField(ctx, logger, fieldVal.Field(i), sf.Type, claim, mnemonics, cache, classID, mnemonic) {
				break
			}
		}
	}

	// Populate sub-claim fields (both for value-bearing and container structs).
	if claim.Size() > 0 {
		matchedSubPropIDs := map[identifier.Identifier]bool{}
		errE := populateFieldsRecursive(ctx, logger, fieldVal, fieldType, claim, mnemonics, matchedSubPropIDs, cache, classID)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// findAndSetValueField searches for and sets a value field in embedded structs.
func findAndSetValueField(
	ctx context.Context, logger *zerolog.Logger,
	structVal reflect.Value, structType reflect.Type,
	claim document.Claim,
	mnemonics map[string]identifier.Identifier,
	cache *baseCache,
	classID identifier.Identifier,
	mnemonic string,
) bool {
	for i := range structType.NumField() {
		sf := structType.Field(i)
		if _, ok := sf.Tag.Lookup("value"); ok {
			vf := structVal.Field(i)
			_ = setSingleFieldFromClaim(ctx, logger, vf, sf.Type, claim, mnemonics, cache, classID, mnemonic)
			return true
		}
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct {
			if findAndSetValueField(ctx, logger, structVal.Field(i), sf.Type, claim, mnemonics, cache, classID, mnemonic) {
				return true
			}
		}
	}
	return false
}

// setStringLikeValue sets a field with string underlying kind from a string value.
func setStringLikeValue(
	fieldVal reflect.Value, fieldType reflect.Type, value string,
	classID identifier.Identifier, mnemonic string, logger *zerolog.Logger,
) errors.E {
	if fieldType.Kind() == reflect.String {
		fieldVal.SetString(value)
		return nil
	}
	logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).
		Str("fieldKind", fieldType.Kind().String()).
		Msg("type mismatch: expected string-like field for string claim")
	return nil
}

// setAmountValue sets a numeric field from an AmountClaim.
func setAmountValue(
	fieldVal reflect.Value, fieldType reflect.Type, claim *document.AmountClaim,
	classID identifier.Identifier, mnemonic string, logger *zerolog.Logger,
) errors.E {
	switch fieldType.Kind() { //nolint:exhaustive
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		f, errE := claim.Amount.Float64(claim.Precision)
		if errE != nil {
			logger.Info().Err(errE).Str("mnemonic", mnemonic).
				Str("classID", classID.String()).Msg("failed to parse amount")
			return nil
		}
		fieldVal.SetInt(int64(f))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		f, errE := claim.Amount.Float64(claim.Precision)
		if errE != nil {
			logger.Info().Err(errE).Str("mnemonic", mnemonic).
				Str("classID", classID.String()).Msg("failed to parse amount")
			return nil
		}
		fieldVal.SetUint(uint64(f))
		return nil
	case reflect.Float32, reflect.Float64:
		f, errE := claim.Amount.Float64(claim.Precision)
		if errE != nil {
			logger.Info().Err(errE).Str("mnemonic", mnemonic).
				Str("classID", classID.String()).Msg("failed to parse amount")
			return nil
		}
		fieldVal.SetFloat(f)
		return nil
	case reflect.String:
		// Amount stored as string.
		fieldVal.SetString(string(claim.Amount))
		return nil
	case reflect.Bool:
		// Type mismatch.
		logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).
			Str("claimType", "AmountClaim").Str("fieldType", fieldType.String()).
			Msg("type mismatch for field")
		return nil
	default:
		logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).
			Str("fieldKind", fieldType.Kind().String()).
			Msg("type mismatch: unsupported kind for AmountClaim")
		return nil
	}
}

// setAmountStructValue handles setting Amount[T] struct types.
// Returns true if the type was recognized.
func setAmountStructValue(
	fieldVal reflect.Value, fieldType reflect.Type, claim document.Claim,
	classID identifier.Identifier, mnemonic string, logger *zerolog.Logger,
) bool {
	amountClaim, ok := claim.(*document.AmountClaim)
	if !ok {
		// Check if this is an Amount type at all.
		if internalCore.AmountTypes[fieldType] {
			logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Str("claimType", fmt.Sprintf("%T", claim)).Msg("expected AmountClaim for Amount field")
			return true
		}
		return false
	}

	if !internalCore.AmountTypes[fieldType] {
		return false
	}

	f, errE := amountClaim.Amount.Float64(amountClaim.Precision)
	if errE != nil {
		logger.Info().Err(errE).Str("mnemonic", mnemonic).Str("classID", classID.String()).Msg("failed to parse amount")
		return true
	}

	// Amount[T] has fields Amount and Precision.
	amountField := fieldVal.Field(0)    // Amount field.
	precisionField := fieldVal.Field(1) // Precision field.

	setNumericField(amountField, f)
	setNumericField(precisionField, amountClaim.Precision)

	return true
}

// setNumericField sets a numeric reflect.Value from a float64.
func setNumericField(v reflect.Value, f float64) {
	switch v.Kind() { //nolint:exhaustive
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(f))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(f))
	case reflect.Float32, reflect.Float64:
		v.SetFloat(f)
	}
}

// setIntervalValue handles setting Interval[T] struct types.
// Returns true if the type was recognized.
func setIntervalValue(
	fieldVal reflect.Value, fieldType reflect.Type,
	claim document.Claim,
	classID identifier.Identifier,
	mnemonic string,
	logger *zerolog.Logger,
) bool {
	// Check for TimeIntervalClaim -> Interval[Time].
	if fieldType == internalCore.TimeIntervalType {
		if tic, ok := claim.(*document.TimeIntervalClaim); ok {
			setTimeIntervalValue(fieldVal, tic)
		} else {
			logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).
				Str("claimType", fmt.Sprintf("%T", claim)).
				Msg("expected TimeIntervalClaim for Interval[Time] field")
		}
		return true
	}

	// Check for AmountIntervalClaim -> Interval[Amount[T]].
	if internalCore.AmountIntervalTypes[fieldType] {
		if aic, ok := claim.(*document.AmountIntervalClaim); ok {
			setAmountIntervalValue(fieldVal, fieldType, aic, classID, mnemonic, logger)
		} else {
			logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).
				Str("claimType", fmt.Sprintf("%T", claim)).
				Msg("expected AmountIntervalClaim for Interval[Amount] field")
		}
		return true
	}

	return false
}

// setTimeIntervalValue sets an Interval[Time] struct from a TimeIntervalClaim.
func setTimeIntervalValue(fieldVal reflect.Value, tic *document.TimeIntervalClaim) {
	// Interval[Time] fields: From, FromIsOpen, FromIsUnknown, FromIsNone, To, ToIsOpen, ToIsUnknown, ToIsNone.
	if tic.From != nil {
		fromPrecision := document.TimePrecisionYear
		if tic.FromPrecision != nil {
			fromPrecision = *tic.FromPrecision
		}
		from := internalCore.Time{
			Time:      mustParseDocTime(*tic.From, fromPrecision),
			Precision: fromPrecision,
		}
		fieldVal.Field(internalCore.IntervalFromIdx).Set(reflect.ValueOf(&from))
	}
	fieldVal.Field(internalCore.IntervalFromIsOpenIdx).SetBool(tic.FromIsOpen)
	fieldVal.Field(internalCore.IntervalFromIsUnknownIdx).SetBool(tic.FromIsUnknown)
	fieldVal.Field(internalCore.IntervalFromIsNoneIdx).SetBool(tic.FromIsNone)

	if tic.To != nil {
		toPrecision := document.TimePrecisionYear
		if tic.ToPrecision != nil {
			toPrecision = *tic.ToPrecision
		}
		to := internalCore.Time{
			Time:      mustParseDocTime(*tic.To, toPrecision),
			Precision: toPrecision,
		}
		fieldVal.Field(internalCore.IntervalToIdx).Set(reflect.ValueOf(&to))
	}
	fieldVal.Field(internalCore.IntervalToIsOpenIdx).SetBool(tic.ToIsOpen)
	fieldVal.Field(internalCore.IntervalToIsUnknownIdx).SetBool(tic.ToIsUnknown)
	fieldVal.Field(internalCore.IntervalToIsNoneIdx).SetBool(tic.ToIsNone)
}

// setAmountIntervalValue sets an Interval[Amount[T]] struct from an AmountIntervalClaim.
func setAmountIntervalValue(
	fieldVal reflect.Value, fieldType reflect.Type,
	aic *document.AmountIntervalClaim,
	classID identifier.Identifier,
	mnemonic string,
	logger *zerolog.Logger,
) {
	// Interval[Amount[T]] fields: From, FromIsOpen, FromIsUnknown, FromIsNone,
	// To, ToIsOpen, ToIsUnknown, ToIsNone. From and To are *Amount[T].
	fromField := fieldVal.Field(internalCore.IntervalFromIdx)
	if aic.From != nil && aic.FromPrecision != nil {
		f, errE := aic.From.Float64(*aic.FromPrecision)
		if errE != nil {
			logger.Info().Err(errE).Str("mnemonic", mnemonic).
				Str("classID", classID.String()).Msg("failed to parse interval from amount")
		} else {
			// Create Amount[T] and set it.
			elemType := fieldType.Field(internalCore.IntervalFromIdx).Type.Elem()
			amountVal := reflect.New(elemType)
			setNumericField(amountVal.Elem().Field(0), f)
			setNumericField(amountVal.Elem().Field(1), *aic.FromPrecision)
			fromField.Set(amountVal)
		}
	}
	fieldVal.Field(internalCore.IntervalFromIsOpenIdx).SetBool(aic.FromIsOpen)
	fieldVal.Field(internalCore.IntervalFromIsUnknownIdx).SetBool(aic.FromIsUnknown)
	fieldVal.Field(internalCore.IntervalFromIsNoneIdx).SetBool(aic.FromIsNone)

	toField := fieldVal.Field(internalCore.IntervalToIdx)
	if aic.To != nil && aic.ToPrecision != nil {
		f, errE := aic.To.Float64(*aic.ToPrecision)
		if errE != nil {
			logger.Info().Err(errE).Str("mnemonic", mnemonic).
				Str("classID", classID.String()).Msg("failed to parse interval to amount")
		} else {
			elemType := fieldType.Field(internalCore.IntervalToIdx).Type.Elem()
			amountVal := reflect.New(elemType)
			setNumericField(amountVal.Elem().Field(0), f)
			setNumericField(amountVal.Elem().Field(1), *aic.ToPrecision)
			toField.Set(amountVal)
		}
	}
	fieldVal.Field(internalCore.IntervalToIsOpenIdx).SetBool(aic.ToIsOpen)
	fieldVal.Field(internalCore.IntervalToIsUnknownIdx).SetBool(aic.ToIsUnknown)
	fieldVal.Field(internalCore.IntervalToIsNoneIdx).SetBool(aic.ToIsNone)
}

// setReferenceValue sets a Ref field from a ReferenceClaim.
func setReferenceValue(
	ctx context.Context,
	fieldVal reflect.Value, fieldType reflect.Type,
	claim *document.ReferenceClaim,
	cache *baseCache,
	classID identifier.Identifier,
	mnemonic string,
	logger *zerolog.Logger,
) errors.E {
	if fieldType == internalCore.RefType {
		base, errE := cache.getBase(ctx, claim.To.ID)
		if errE != nil {
			return errE
		}
		if base != nil {
			fieldVal.Set(reflect.ValueOf(internalCore.Ref{ID: slices.Clone(base)}))
		}
		return nil
	}
	logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Str("fieldType", fieldType.String()).Msg("type mismatch: expected Ref for ReferenceClaim")
	return nil
}

// setTimeValue sets a time-related field from a TimeClaim.
func setTimeValue(
	fieldVal reflect.Value, fieldType reflect.Type, claim *document.TimeClaim,
	classID identifier.Identifier, mnemonic string, logger *zerolog.Logger,
) errors.E {
	// If it's a string kind (document.Time is a string type), set directly.
	if fieldType.Kind() == reflect.String {
		fieldVal.SetString(string(claim.Time))
		return nil
	}
	logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Str("fieldType", fieldType.String()).Msg("type mismatch for TimeClaim")
	return nil
}

// setBoolValue sets a boolean field.
func setBoolValue(fieldVal reflect.Value, fieldType reflect.Type, value bool, classID identifier.Identifier, mnemonic string, logger *zerolog.Logger) errors.E {
	if fieldType.Kind() == reflect.Bool {
		fieldVal.SetBool(value)
		return nil
	}
	logger.Info().Str("mnemonic", mnemonic).Str("classID", classID.String()).Str("fieldType", fieldType.String()).Msg("type mismatch: expected bool for None/Unknown claim")
	return nil
}

// logUnmatchedClaims logs info about claims whose property IDs don't match any struct field.
func logUnmatchedClaims(
	_ context.Context, logger *zerolog.Logger,
	doc *document.D,
	matchedPropIDs map[identifier.Identifier]bool,
	propToMnemonic map[identifier.Identifier]string,
) {
	if doc.Claims == nil {
		return
	}
	for claim := range doc.Claims.AllClaims() {
		propID := claim.GetProp().ID
		if matchedPropIDs[propID] {
			continue
		}
		name := propID.String()
		if m, ok := propToMnemonic[propID]; ok {
			name = m
		}
		logger.Info().Str("docID", doc.ID.String()).Str("property", name).Msg("unmatched claim property")
	}
}

// mustParseDocTime parses a document.Time into a time.Time with the given precision.
// Returns zero time on parse error.
func mustParseDocTime(t document.Time, precision document.TimePrecision) time.Time {
	parsed, errE := t.Time(precision, nil)
	if errE != nil {
		return time.Time{}
	}
	return parsed
}
