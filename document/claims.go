// Package document provides data structures and operations for PeerDB documents and their claims.
package document

import (
	"cmp"
	"fmt"
	"iter"
	"math"
	"slices"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// sortByConfidence sorts claims in decreasing confidence order.
func sortByConfidence(claims []Claim) {
	slices.SortFunc(claims, func(a, b Claim) int {
		// Reverse order: higher confidence first.
		return cmp.Compare(b.GetConfidence(), a.GetConfidence())
	})
}

// Claim is the interface for all claim types in PeerDB documents.
type Claim interface {
	ClaimsContainer

	GetConfidence() Confidence
}

// Claims is the interface for types that hold and manipulate a collection of claims.
type Claims interface {
	Visit(visitor Visitor) errors.E
	Get(propID identifier.Identifier) []Claim
	Remove(propID identifier.Identifier) []Claim
	GetByID(id identifier.Identifier) Claim
	RemoveByID(id identifier.Identifier) Claim
	Add(claim Claim) errors.E
	Size() int
	AllClaims() iter.Seq[Claim]
	Validate() errors.E
}

var (
	_ Claim = (*IdentifierClaim)(nil)
	_ Claim = (*StringClaim)(nil)
	_ Claim = (*HTMLClaim)(nil)
	_ Claim = (*AmountClaim)(nil)
	_ Claim = (*AmountIntervalClaim)(nil)
	_ Claim = (*TimeClaim)(nil)
	_ Claim = (*TimeIntervalClaim)(nil)
	_ Claim = (*LinkClaim)(nil)
	_ Claim = (*ReferenceClaim)(nil)
	_ Claim = (*HasClaim)(nil)
	_ Claim = (*NoneClaim)(nil)
	_ Claim = (*UnknownClaim)(nil)
)

// GetClaimsOfType returns all claims of the concrete type T matching the given property ID,
// sorted by decreasing confidence.
//
// It operates like Claims.Get but returns the concrete claim type instead of the Claim interface.
//
// Because Go does not support generic interface methods, this is a top-level function.
func GetClaimsOfType[T Claim](claims Claims, propID identifier.Identifier) []T {
	// Get already returns claims sorted by decreasing confidence.
	all := claims.Get(propID)
	result := make([]T, 0, len(all))
	for _, c := range all {
		if typed, ok := c.(T); ok {
			result = append(result, typed)
		}
	}
	return result
}

// GetBestClaimOfType returns the best (one with highest confidence) claim of the concrete type
// T matching the given property ID, or the zero value of T if no matching claim is found.
//
// Because Go does not support generic interface methods, this is a top-level function.
func GetBestClaimOfType[T Claim](claims Claims, propID identifier.Identifier) T { //nolint:ireturn
	// The best claim is really the first one because GetClaimsOfType returns claims in decreasing confidence.
	for _, c := range GetClaimsOfType[T](claims, propID) {
		return c
	}
	return *new(T)
}

// GetAllClaimsOfType returns all claims of the concrete type T,
// sorted by decreasing confidence.
//
// Because Go does not support generic interface methods, this is a top-level function.
func GetAllClaimsOfType[T Claim](claims Claims) []T {
	var result []T
	for c := range claims.AllClaims() {
		if typed, ok := c.(T); ok {
			result = append(result, typed)
		}
	}
	slices.SortFunc(result, func(a, b T) int {
		// Reverse order: higher confidence first.
		return cmp.Compare(b.GetConfidence(), a.GetConfidence())
	})
	return result
}

// GetAllClaimsOfTypeWithConfidence returns all claims of the concrete type T,
// sorted by decreasing confidence, that have confidence equal to or higher than
// the specified minimum confidence.
//
// If confidence is 0, it defaults to LowConfidence.
//
// Because Go does not support generic interface methods, this is a top-level function.
// TODO: Support also negation claims (i.e., those with negative confidence).
func GetAllClaimsOfTypeWithConfidence[T Claim](claims Claims, confidence Confidence) []T {
	if confidence == 0 {
		confidence = LowConfidence
	}
	all := GetAllClaimsOfType[T](claims)
	result := make([]T, 0, len(all))
	for _, c := range all {
		if Claim(c).GetConfidence() >= confidence {
			result = append(result, c)
		} else {
			// Because GetAllClaimsOfType returns claims sorted by decreasing confidence, we can break here.
			break
		}
	}
	return result
}

// GetClaimsOfTypeWithConfidence returns all claims of the concrete type T matching the given
// property ID, sorted by decreasing confidence, that have confidence equal to or higher than
// the specified minimum confidence.
//
// If confidence is 0, it defaults to LowConfidence.
//
// Because Go does not support generic interface methods, this is a top-level function.
// TODO: Support also negation claims (i.e., those with negative confidence).
func GetClaimsOfTypeWithConfidence[T Claim](claims Claims, propID identifier.Identifier, confidence Confidence) []T {
	if confidence == 0 {
		confidence = LowConfidence
	}
	all := GetClaimsOfType[T](claims, propID)
	result := make([]T, 0, len(all))
	for _, c := range all {
		if Claim(c).GetConfidence() >= confidence {
			result = append(result, c)
		} else {
			// Because GetClaimsOfType returns claims sorted by decreasing confidence, we can break here.
			break
		}
	}
	return result
}

// Well-known IDs computed from the core namespace.
//
//nolint:gochecknoglobals
var (
	// coreNamespace must match core.Namespace. We cannot import core here due to import cycle.
	coreNamespace     = "core.peerdb.org"
	listPropID        = identifier.From(coreNamespace, "LIST")
	orderInListPropID = identifier.From(coreNamespace, "ORDER_IN_LIST")
)

// GetClaimsListsOfType groups claims of the concrete type T matching the given property ID
// by their LIST meta-claim and sorts within each list by the ORDER_IN_LIST meta-claim.
// Returns a slice of lists, where each list is a slice of claims sorted by order.
//
// Because Go does not support generic interface methods, this is a top-level function.
// TODO: Handle sub-lists. Children lists should be nested and not just added as additional lists to the list of lists.
// TODO: Sort lists between themselves by (average) confidence?
func GetClaimsListsOfType[T Claim](claims Claims, propID identifier.Identifier) [][]T {
	all := GetClaimsOfType[T](claims, propID)
	if len(all) == 0 {
		return nil
	}

	type entry struct {
		claim T
		order float64
	}

	claimsPerList := map[string][]entry{}
	for _, c := range all {
		listID := "none" //nolint:goconst
		if listClaim := GetBestClaimOfType[*IdentifierClaim](Claim(c), listPropID); listClaim != nil {
			listID = listClaim.Value
		}
		order := math.MaxFloat64
		if orderClaim := GetBestClaimOfType[*AmountClaim](Claim(c), orderInListPropID); orderClaim != nil {
			f, errE := orderClaim.Amount.Float64(0)
			if errE == nil {
				order = f
			}
		}
		claimsPerList[listID] = append(claimsPerList[listID], entry{claim: c, order: order})
	}

	result := make([][]T, 0, len(claimsPerList))
	for _, entries := range claimsPerList {
		slices.SortFunc(entries, func(a, b entry) int {
			return cmp.Compare(a.order, b.order)
		})
		list := make([]T, 0, len(entries))
		for _, e := range entries {
			list = append(list, e.claim)
		}
		result = append(result, list)
	}
	return result
}

// ClaimTypes organizes claims by their type.
type ClaimTypes struct {
	Identifier     IdentifierClaims     `exhaustruct:"optional" json:"id,omitempty"`
	String         StringClaims         `exhaustruct:"optional" json:"string,omitempty"`
	HTML           HTMLClaims           `exhaustruct:"optional" json:"html,omitempty"`
	Amount         AmountClaims         `exhaustruct:"optional" json:"amount,omitempty"`
	AmountInterval AmountIntervalClaims `exhaustruct:"optional" json:"amountInterval,omitempty"`
	Time           TimeClaims           `exhaustruct:"optional" json:"time,omitempty"`
	TimeInterval   TimeIntervalClaims   `exhaustruct:"optional" json:"timeInterval,omitempty"`
	Link           LinkClaims           `exhaustruct:"optional" json:"link,omitempty"`
	Reference      ReferenceClaims      `exhaustruct:"optional" json:"ref,omitempty"`
	Has            HasClaims            `exhaustruct:"optional" json:"has,omitempty"`
	None           NoneClaims           `exhaustruct:"optional" json:"none,omitempty"`
	Unknown        UnknownClaims        `exhaustruct:"optional" json:"unknown,omitempty"`
}

var _ Claims = (*ClaimTypes)(nil)

// Add adds a claim to the appropriate typed slice based on the claim's type.
func (c *ClaimTypes) Add(claim Claim) errors.E {
	if claim == nil {
		errE := errors.New("claim type not supported")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", claim)
		return errE
	}

	claimID := claim.GetID()
	if c.GetByID(claimID) != nil {
		errE := errors.New("claim with ID already exists")
		errors.Details(errE)["id"] = claimID
		return errE
	}

	switch cl := claim.(type) {
	case *IdentifierClaim:
		c.Identifier = append(c.Identifier, *cl)
	case *StringClaim:
		c.String = append(c.String, *cl)
	case *HTMLClaim:
		c.HTML = append(c.HTML, *cl)
	case *AmountClaim:
		c.Amount = append(c.Amount, *cl)
	case *AmountIntervalClaim:
		c.AmountInterval = append(c.AmountInterval, *cl)
	case *TimeClaim:
		c.Time = append(c.Time, *cl)
	case *TimeIntervalClaim:
		c.TimeInterval = append(c.TimeInterval, *cl)
	case *LinkClaim:
		c.Link = append(c.Link, *cl)
	case *ReferenceClaim:
		c.Reference = append(c.Reference, *cl)
	case *HasClaim:
		c.Has = append(c.Has, *cl)
	case *NoneClaim:
		c.None = append(c.None, *cl)
	case *UnknownClaim:
		c.Unknown = append(c.Unknown, *cl)
	default:
		errE := errors.New("claim type not supported")
		errors.Details(errE)["type"] = fmt.Sprintf("%T", claim)
		return errE
	}

	return nil
}

// Get returns all claims with the given property ID, sorted by decreasing confidence.
func (c *ClaimTypes) Get(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Keep,
		Result: []Claim{},
	}
	_ = c.Visit(&v)
	sortByConfidence(v.Result)
	return v.Result
}

// GetByID returns the claim with the given ID.
func (c *ClaimTypes) GetByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: KeepAndStop,
		Result: nil,
	}
	_ = c.Visit(&v)
	return v.Result
}

// Remove removes and returns all claims with the given property ID.
func (c *ClaimTypes) Remove(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Drop,
		Result: []Claim{},
	}
	_ = c.Visit(&v)
	return v.Result
}

// RemoveByID removes and returns the claim with the given ID.
func (c *ClaimTypes) RemoveByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: DropAndStop,
		Result: nil,
	}
	_ = c.Visit(&v)
	return v.Result
}

// Size returns the total number of claims across all types.
func (c *ClaimTypes) Size() int {
	if c == nil {
		return 0
	}

	s := 0
	s += len(c.Identifier)
	s += len(c.String)
	s += len(c.HTML)
	s += len(c.Amount)
	s += len(c.AmountInterval)
	s += len(c.Time)
	s += len(c.TimeInterval)
	s += len(c.Link)
	s += len(c.Reference)
	s += len(c.Has)
	s += len(c.None)
	s += len(c.Unknown)
	return s
}

// AllClaims returns an iterator over all claims.
func (c *ClaimTypes) AllClaims() iter.Seq[Claim] {
	return func(yield func(Claim) bool) {
		_ = c.Visit(&AllClaimsVisitor{Yield: yield})
	}
}

// AllClaimsWithMeta returns an iterator over all claims, including meta claims.
func (c *ClaimTypes) AllClaimsWithMeta() iter.Seq[Claim] {
	return func(yield func(Claim) bool) {
		_ = c.Visit(&AllClaimsWithMetaVisitor{Yield: yield, stopped: false})
	}
}

// Validate checks that all claims are valid.
func (c *ClaimTypes) Validate() errors.E {
	claims := map[identifier.Identifier]bool{}
	for claim := range c.AllClaimsWithMeta() {
		errE := claim.Validate()
		if errE != nil {
			return errE
		}
		claimID := claim.GetID()
		if claims[claimID] {
			errE := errors.New("duplicate claim ID")
			errors.Details(errE)["id"] = claimID
			return errE
		}
		claims[claimID] = true
	}
	return nil
}

type (
	// IdentifierClaims is a slice of IdentifierClaim.
	IdentifierClaims = []IdentifierClaim
	// StringClaims is a slice of StringClaim.
	StringClaims = []StringClaim
	// HTMLClaims is a slice of HTMLClaim.
	HTMLClaims = []HTMLClaim
	// AmountClaims is a slice of AmountClaim.
	AmountClaims = []AmountClaim
	// AmountIntervalClaims is a slice of AmountIntervalClaim.
	AmountIntervalClaims = []AmountIntervalClaim
	// TimeClaims is a slice of TimeClaim.
	TimeClaims = []TimeClaim
	// TimeIntervalClaims is a slice of TimeIntervalClaim.
	TimeIntervalClaims = []TimeIntervalClaim
	// LinkClaims is a slice of LinkClaim.
	LinkClaims = []LinkClaim
	// ReferenceClaims is a slice of ReferenceClaim.
	ReferenceClaims = []ReferenceClaim
	// HasClaims is a slice of HasClaim.
	HasClaims = []HasClaim
	// NoneClaims is a slice of NoneClaim.
	NoneClaims = []NoneClaim
	// UnknownClaims is a slice of UnknownClaim.
	UnknownClaims = []UnknownClaim
)

// CoreClaim contains fields common to all claim types.
type CoreClaim struct {
	ID         identifier.Identifier `                       json:"id"`
	Confidence Confidence            `                       json:"confidence"`
	Meta       *ClaimTypes           `exhaustruct:"optional" json:"meta,omitempty"`
}

// GetID returns the claim's identifier.
func (cc *CoreClaim) GetID() identifier.Identifier {
	return cc.ID
}

// GetConfidence returns the claim's confidence score.
func (cc *CoreClaim) GetConfidence() Confidence {
	return cc.Confidence
}

// Validate checks that the claim has valid confidence and that meta claims are valid.
func (cc *CoreClaim) Validate() errors.E {
	if math.IsInf(float64(cc.Confidence), 0) || math.IsNaN(float64(cc.Confidence)) || cc.Confidence < -1 || cc.Confidence > 1 {
		return errors.New("confidence out of range [-1, 1]")
	}

	if cc.Meta != nil {
		return cc.Meta.Validate()
	}

	return nil
}

// Visit applies a visitor to the claim's metadata claims.
func (cc *CoreClaim) Visit(visitor Visitor) errors.E {
	if cc.Meta != nil {
		err := cc.Meta.Visit(visitor)
		if err != nil {
			return err
		}
		// If meta claims became empty after visiting, we set them to nil.
		if cc.Meta.Size() == 0 {
			cc.Meta = nil
		}
	}
	return nil
}

// Get returns all metadata claims with the given property ID, sorted by decreasing confidence.
func (cc *CoreClaim) Get(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Keep,
		Result: []Claim{},
	}
	_ = cc.Visit(&v)
	sortByConfidence(v.Result)
	return v.Result
}

// Remove removes and returns all metadata claims with the given property ID.
func (cc *CoreClaim) Remove(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Drop,
		Result: []Claim{},
	}
	_ = cc.Visit(&v)
	return v.Result
}

// GetByID returns the metadata claim with the given ID.
func (cc *CoreClaim) GetByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: KeepAndStop,
		Result: nil,
	}
	_ = cc.Visit(&v)
	return v.Result
}

// RemoveByID removes and returns the metadata claim with the given ID.
func (cc *CoreClaim) RemoveByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: DropAndStop,
		Result: nil,
	}
	_ = cc.Visit(&v)
	return v.Result
}

// Add adds a metadata claim to the claim.
func (cc *CoreClaim) Add(claim Claim) errors.E {
	claimID := claim.GetID()
	if cc.ID == claimID || cc.GetByID(claimID) != nil {
		errE := errors.New("claim with ID already exists")
		errors.Details(errE)["id"] = claimID
		return errE
	}
	if cc.Meta == nil {
		cc.Meta = new(ClaimTypes)
	}
	return cc.Meta.Add(claim)
}

// Size returns the number of metadata claims in the claim.
func (cc *CoreClaim) Size() int {
	return cc.Meta.Size()
}

// AllClaims returns an iterator over all metadata claims.
func (cc *CoreClaim) AllClaims() iter.Seq[Claim] {
	return cc.Meta.AllClaims()
}

// Reference represents a reference to another document.
type Reference struct {
	ID identifier.Identifier `json:"id"`
}

// GetReference returns a reference with the given values converted to an ID.
func GetReference(values ...string) Reference {
	return Reference{
		ID: identifier.From(values...),
	}
}

// IdentifierClaim represents a claim with a string identifier value.
type IdentifierClaim struct {
	CoreClaim

	Prop  Reference `json:"prop"`
	Value string    `json:"value"`
}

// Validate checks that the identifier claim has a non-empty value and valid confidence.
func (c *IdentifierClaim) Validate() errors.E {
	errE := c.CoreClaim.Validate()
	if errE != nil {
		return errE
	}
	if c.Value == "" {
		return errors.New("empty value")
	}

	return nil
}

// StringClaim represents a claim with a plain string value.
//
// Language of the string, if any, is specified as a meta claim.
type StringClaim struct {
	CoreClaim

	Prop   Reference `json:"prop"`
	String string    `json:"string"`
}

// Validate checks that the string claim has a non-empty string and valid confidence.
func (c *StringClaim) Validate() errors.E {
	errE := c.CoreClaim.Validate()
	if errE != nil {
		return errE
	}
	if c.String == "" {
		return errors.New("empty string")
	}

	return nil
}

// HTMLClaim represents a claim with HTML text content.
//
// Language of the string, if any, is specified as a meta claim.
type HTMLClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
	HTML string    `json:"html"`
}

// Validate checks that the HTML claim has non-empty HTML and valid confidence.
func (c *HTMLClaim) Validate() errors.E {
	errE := c.CoreClaim.Validate()
	if errE != nil {
		return errE
	}
	if c.HTML == "" {
		return errors.New("empty HTML")
	}

	return nil
}

// AmountClaim represents a claim for numeric amount and precision.
//
// Precision is represented as number so that round(amount / precision) * precision == amount.
// For example, 100 represents two digits precision, 60 represents
// minute precision for seconds.
//
// Infinite or NaN amounts are not supported.
//
// Unit of the amount, if any, is specified as a meta claim.
type AmountClaim struct {
	CoreClaim

	Prop      Reference `json:"prop"`
	Amount    Amount    `json:"amount"`
	Precision float64   `json:"precision"`
}

// Validate checks that the amount claim has valid amount, precision, and confidence.
func (c *AmountClaim) Validate() errors.E {
	errE := c.CoreClaim.Validate()
	if errE != nil {
		return errE
	}
	if math.IsInf(c.Precision, 0) || math.IsNaN(c.Precision) || c.Precision <= 0 {
		return errors.New("Precision must be a finite positive number")
	}

	return c.Amount.Validate(c.Precision)
}

// AmountIntervalClaim represents a claim for numeric amount interval.
//
// Infinite or NaN amount interval bounds are not supported.
//
// Unit of the amount interval bounds, if any, is specified as a meta claim.
//
// Only one of FromIs* fields can be set at a time.
// Exactly one of From (non-nil), FromIsUnknown, or FromIsNone must be set.
// From and FromPrecision must be set together or both nil.
// If FromIsUnknown or FromIsNone is true, From and FromPrecision must be nil.
//
// Only one of ToIs* fields can be set at a time.
// Exactly one of To (non-nil), ToIsUnknown, or ToIsNone must be set.
// To and ToPrecision must be set together or both nil.
// If ToIsUnknown or ToIsNone is true, To and ToPrecision must be nil.
type AmountIntervalClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`

	From          *Amount  `json:"from,omitempty"`
	FromPrecision *float64 `json:"fromPrecision,omitempty"`
	FromIsOpen    bool     `json:"fromIsOpen,omitempty"`
	FromIsUnknown bool     `json:"fromIsUnknown,omitempty"`
	FromIsNone    bool     `json:"fromIsNone,omitempty"`

	To          *Amount  `json:"to,omitempty"`
	ToPrecision *float64 `json:"toPrecision,omitempty"`
	ToIsClosed  bool     `json:"toIsClosed,omitempty"`
	ToIsUnknown bool     `json:"toIsUnknown,omitempty"`
	ToIsNone    bool     `json:"toIsNone,omitempty"`
}

// Validate checks that the amount interval claim has valid bounds and valid confidence.
func (c *AmountIntervalClaim) Validate() errors.E {
	errE := c.CoreClaim.Validate()
	if errE != nil {
		return errE
	}

	fromIsCount := 0
	if c.FromIsOpen {
		fromIsCount++
	}
	if c.FromIsUnknown {
		fromIsCount++
	}
	if c.FromIsNone {
		fromIsCount++
	}
	if fromIsCount > 1 {
		return errors.New("only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")
	}
	if (c.From == nil) != (c.FromPrecision == nil) {
		return errors.New("From and FromPrecision must be set together")
	}
	if c.From == nil && !c.FromIsUnknown && !c.FromIsNone {
		return errors.New("one of From, FromIsUnknown, or FromIsNone must be set")
	}
	if c.From != nil && (c.FromIsUnknown || c.FromIsNone) {
		return errors.New("From must not be set when FromIsUnknown or FromIsNone is true")
	}
	if c.FromPrecision != nil {
		if math.IsInf(*c.FromPrecision, 0) || math.IsNaN(*c.FromPrecision) || *c.FromPrecision <= 0 {
			return errors.New("FromPrecision must be finite positive number")
		}
		errE := c.From.Validate(*c.FromPrecision)
		if errE != nil {
			return errE
		}
	}

	toIsCount := 0
	if c.ToIsClosed {
		toIsCount++
	}
	if c.ToIsUnknown {
		toIsCount++
	}
	if c.ToIsNone {
		toIsCount++
	}
	if toIsCount > 1 {
		return errors.New("only one of ToIsClosed, ToIsUnknown, ToIsNone can be set")
	}
	if (c.To == nil) != (c.ToPrecision == nil) {
		return errors.New("To and ToPrecision must be set together")
	}
	if c.To == nil && !c.ToIsUnknown && !c.ToIsNone {
		return errors.New("one of To, ToIsUnknown, or ToIsNone must be set")
	}
	if c.To != nil && (c.ToIsUnknown || c.ToIsNone) {
		return errors.New("To must not be set when ToIsUnknown or ToIsNone is true")
	}
	if c.ToPrecision != nil {
		if math.IsInf(*c.ToPrecision, 0) || math.IsNaN(*c.ToPrecision) || *c.ToPrecision <= 0 {
			return errors.New("ToPrecision must be finite positive number")
		}
		errE := c.To.Validate(*c.ToPrecision)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// TimeClaim represents a claim for timestamp and precision.
type TimeClaim struct {
	CoreClaim

	Prop      Reference     `json:"prop"`
	Timestamp Timestamp     `json:"time"`
	Precision TimePrecision `json:"precision"`
}

// Validate checks that the time claim has a valid precision, timestamp, and valid confidence.
func (t *TimeClaim) Validate() errors.E {
	errE := t.CoreClaim.Validate()
	if errE != nil {
		return errE
	}
	if t.Precision < TimePrecisionGigaYears || t.Precision > TimePrecisionNanosecond {
		return errors.New("unknown Precision")
	}

	return t.Timestamp.Validate(t.Precision)
}

// TimeIntervalClaim represents a claim for timestamp interval.
type TimeIntervalClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`

	From          *Timestamp     `json:"from,omitempty"`
	FromPrecision *TimePrecision `json:"fromPrecision,omitempty"`
	FromIsOpen    bool           `json:"fromIsOpen,omitempty"`
	FromIsUnknown bool           `json:"fromIsUnknown,omitempty"`
	FromIsNone    bool           `json:"fromIsNone,omitempty"`

	To          *Timestamp     `json:"to,omitempty"`
	ToPrecision *TimePrecision `json:"toPrecision,omitempty"`
	ToIsClosed  bool           `json:"toIsClosed,omitempty"`
	ToIsUnknown bool           `json:"toIsUnknown,omitempty"`
	ToIsNone    bool           `json:"toIsNone,omitempty"`
}

// Validate checks that the time interval claim has valid bounds and valid confidence.
func (c *TimeIntervalClaim) Validate() errors.E {
	errE := c.CoreClaim.Validate()
	if errE != nil {
		return errE
	}

	fromIsCount := 0
	if c.FromIsOpen {
		fromIsCount++
	}
	if c.FromIsUnknown {
		fromIsCount++
	}
	if c.FromIsNone {
		fromIsCount++
	}
	if fromIsCount > 1 {
		return errors.New("only one of FromIsOpen, FromIsUnknown, FromIsNone can be set")
	}
	if (c.From == nil) != (c.FromPrecision == nil) {
		return errors.New("From and FromPrecision must be set together")
	}
	if c.From == nil && !c.FromIsUnknown && !c.FromIsNone {
		return errors.New("one of From, FromIsUnknown, or FromIsNone must be set")
	}
	if c.From != nil && (c.FromIsUnknown || c.FromIsNone) {
		return errors.New("From must not be set when FromIsUnknown or FromIsNone is true")
	}
	if c.FromPrecision != nil {
		if *c.FromPrecision < TimePrecisionGigaYears || *c.FromPrecision > TimePrecisionNanosecond {
			return errors.New("unknown FromPrecision")
		}
		errE := c.From.Validate(*c.FromPrecision)
		if errE != nil {
			return errE
		}
	}

	toIsCount := 0
	if c.ToIsClosed {
		toIsCount++
	}
	if c.ToIsUnknown {
		toIsCount++
	}
	if c.ToIsNone {
		toIsCount++
	}
	if toIsCount > 1 {
		return errors.New("only one of ToIsClosed, ToIsUnknown, ToIsNone can be set")
	}
	if (c.To == nil) != (c.ToPrecision == nil) {
		return errors.New("To and ToPrecision must be set together")
	}
	if c.To == nil && !c.ToIsUnknown && !c.ToIsNone {
		return errors.New("one of To, ToIsUnknown, or ToIsNone must be set")
	}
	if c.To != nil && (c.ToIsUnknown || c.ToIsNone) {
		return errors.New("To must not be set when ToIsUnknown or ToIsNone is true")
	}
	if c.ToPrecision != nil {
		if *c.ToPrecision < TimePrecisionGigaYears || *c.ToPrecision > TimePrecisionNanosecond {
			return errors.New("unknown ToPrecision")
		}
		errE := c.To.Validate(*c.ToPrecision)
		if errE != nil {
			return errE
		}
	}

	return nil
}

// LinkClaim represents a claim with an IRI (Internationalized Resource Identifier) value.
type LinkClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
	IRI  string    `json:"iri"`
}

// Validate checks that the link claim has a non-empty IRI and valid confidence.
func (c *LinkClaim) Validate() errors.E {
	errE := c.CoreClaim.Validate()
	if errE != nil {
		return errE
	}
	if c.IRI == "" {
		return errors.New("empty IRI")
	}

	return nil
}

// ReferenceClaim represents a claim that relates this document to another document.
type ReferenceClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
	To   Reference `json:"to"`
}

// HasClaim represents a claim with just a property.
//
// It can also be used for nested claims.
type HasClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
}

// NoneClaim represents a claim that explicitly states no value exists for a property.
type NoneClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
}

// UnknownClaim represents a claim where the value for a property is known to exist but is unknown.
type UnknownClaim struct {
	CoreClaim

	Prop Reference `json:"prop"`
}
