package document

import (
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// D represents a PeerDB document.
type D struct {
	CoreDocument

	Mnemonic Mnemonic    `exhaustruct:"optional" json:"mnemonic,omitempty"`
	Claims   *ClaimTypes `exhaustruct:"optional" json:"claims,omitempty"`
}

// ClaimsContainer defines the interface for types that can hold and manipulate claims.
type ClaimsContainer interface {
	GetID() identifier.Identifier
	Visit(visitor Visitor) errors.E
	Get(propID identifier.Identifier) []Claim
	Remove(propID identifier.Identifier) []Claim
	GetByID(id identifier.Identifier) Claim
	RemoveByID(id identifier.Identifier) Claim
	Add(claim Claim) errors.E
	Size() int
	AllClaims() []Claim
}

var _ ClaimsContainer = (*D)(nil)

// Reference returns a Reference to this document.
func (d D) Reference() Reference {
	return Reference{
		ID: &d.ID,
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

// Get returns all claims with the given property ID.
func (d *D) Get(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Keep,
		Result: []Claim{},
	}
	_ = d.Visit(&v)
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
		return errors.Errorf(`claim with ID "%s" already exists`, claimID)
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

// AllClaims returns all claims in the document as a flat slice.
func (d *D) AllClaims() []Claim {
	v := AllClaimsVisitor{
		Result: []Claim{},
	}
	_ = d.Visit(&v)
	return v.Result
}

// MergeFrom merges claims from one or more other documents into this document.
func (d *D) MergeFrom(other ...*D) errors.E {
	// TODO: What to do about duplicate equal claims (e.g., same NAME claim)?
	//       Skip them? What is an equal claim, what if just metadata is different?

	for _, o := range other {
		for _, claim := range o.AllClaims() {
			// Add makes sure that there are no duplicate claim IDs.
			err := d.Add(claim)
			if err != nil {
				return err
			}
		}
	}
	// TODO: What to do about scores after merging?
	d.Score = LowConfidence
	d.Scores = nil
	return nil
}
