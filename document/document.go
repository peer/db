package document

import (
	"iter"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// CoreDocument contains the core fields present in all PeerDB documents.
type CoreDocument struct {
	ID   identifier.Identifier `json:"id"`
	Base []string              `json:"base"`
}

// GetID returns the document's identifier.
func (d CoreDocument) GetID() identifier.Identifier {
	return d.ID
}

// Validate checks that the document has a valid identifier.
func (d CoreDocument) Validate() errors.E {
	expectedID := identifier.From(d.Base...)
	if d.ID != expectedID {
		errE := errors.New("invalid ID")
		errors.Details(errE)["id"] = d.ID.String()
		errors.Details(errE)["expected"] = expectedID.String()
		return errE
	}
	return nil
}

// D represents a PeerDB document.
//
//nolint:recvcheck
type D struct {
	CoreDocument

	Claims *ClaimTypes `exhaustruct:"optional" json:"claims,omitempty"`
}

// ClaimsContainer defines the interface for types that can hold and manipulate claims.
type ClaimsContainer interface {
	Claims

	GetID() identifier.Identifier
}

var _ ClaimsContainer = (*D)(nil)

// Reference returns a Reference to this document.
func (d D) Reference() Reference {
	return Reference{
		ID: d.ID,
	}
}

// Visit applies a visitor to the document's claims.
func (d *D) Visit(visitor Visitor) errors.E {
	if d.Claims != nil {
		err := d.Claims.Visit(visitor)
		if err != nil {
			return err
		}
		// If claims became empty after visiting, we set them to nil.
		if d.Claims.Size() == 0 {
			d.Claims = nil
		}
	}
	return nil
}

// Get returns all claims with the given property ID, sorted by decreasing confidence.
func (d *D) Get(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Keep,
		Result: []Claim{},
	}
	_ = d.Visit(&v)
	sortByConfidence(v.Result)
	return v.Result
}

// Remove removes and returns all claims with the given property ID.
func (d *D) Remove(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Drop,
		Result: []Claim{},
	}
	_ = d.Visit(&v)
	return v.Result
}

// GetByID returns the claim with the given ID.
func (d *D) GetByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: KeepAndStop,
		Result: nil,
	}
	_ = d.Visit(&v)
	return v.Result
}

// RemoveByID removes and returns the claim with the given ID.
func (d *D) RemoveByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: DropAndStop,
		Result: nil,
	}
	_ = d.Visit(&v)
	return v.Result
}

// Add adds a claim to the document, ensuring no duplicate claim IDs exist.
func (d *D) Add(claim Claim) errors.E {
	if claimID := claim.GetID(); d.GetByID(claimID) != nil {
		errE := errors.New("claim with ID already exists")
		errors.Details(errE)["id"] = claimID
		return errE
	}
	if d.Claims == nil {
		d.Claims = &ClaimTypes{}
	}
	return d.Claims.Add(claim)
}

// Size returns the total number of claims in the document.
func (d *D) Size() int {
	return d.Claims.Size()
}

// AllClaims returns an iterator over all claims in the document.
func (d *D) AllClaims() iter.Seq[Claim] {
	return func(yield func(Claim) bool) {
		_ = d.Visit(&AllClaimsVisitor{Yield: yield})
	}
}

// Validate checks that the document has a valid identifier and that all claims are valid.
func (d *D) Validate() errors.E {
	errE := d.CoreDocument.Validate()
	if errE != nil {
		return errE
	}
	if d.Claims != nil {
		return d.Claims.Validate()
	}
	return nil
}

// MergeFrom merges claims from one or more other documents into this document.
func (d *D) MergeFrom(other ...*D) errors.E {
	// TODO: What to do about duplicate equal claims (e.g., same NAME claim)?
	//       Skip them? What is an equal claim, what if just metadata is different?

	for _, o := range other {
		for claim := range o.AllClaims() {
			// Add makes sure that there are no duplicate claim IDs.
			err := d.Add(claim)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
