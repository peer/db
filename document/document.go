package document

import (
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/internal/es"
)

type D struct {
	CoreDocument

	Mnemonic Mnemonic    `exhaustruct:"optional" json:"mnemonic,omitempty"`
	Claims   *ClaimTypes `exhaustruct:"optional" json:"claims,omitempty"`
}

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

func (d D) Reference() Reference {
	return Reference{
		ID:    &d.ID,
		Score: d.Score,
	}
}

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

func (d *D) Get(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Keep,
		Result: []Claim{},
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *D) Remove(propID identifier.Identifier) []Claim {
	v := GetByPropIDVisitor{
		ID:     propID,
		Action: Drop,
		Result: []Claim{},
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *D) GetByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: KeepAndStop,
		Result: nil,
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *D) RemoveByID(id identifier.Identifier) Claim { //nolint:ireturn
	v := GetByIDVisitor{
		ID:     id,
		Action: DropAndStop,
		Result: nil,
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *D) Add(claim Claim) errors.E {
	if claimID := claim.GetID(); d.GetByID(claimID) != nil {
		return errors.Errorf(`claim with ID "%s" already exists`, claimID)
	}
	if d.Claims == nil {
		d.Claims = &ClaimTypes{}
	}
	switch c := claim.(type) { //nolint:dupl
	case *IdentifierClaim:
		d.Claims.Identifier = append(d.Claims.Identifier, *c)
	case *ReferenceClaim:
		d.Claims.Reference = append(d.Claims.Reference, *c)
	case *TextClaim:
		d.Claims.Text = append(d.Claims.Text, *c)
	case *StringClaim:
		d.Claims.String = append(d.Claims.String, *c)
	case *AmountClaim:
		d.Claims.Amount = append(d.Claims.Amount, *c)
	case *AmountRangeClaim:
		d.Claims.AmountRange = append(d.Claims.AmountRange, *c)
	case *RelationClaim:
		d.Claims.Relation = append(d.Claims.Relation, *c)
	case *FileClaim:
		d.Claims.File = append(d.Claims.File, *c)
	case *NoValueClaim:
		d.Claims.NoValue = append(d.Claims.NoValue, *c)
	case *UnknownValueClaim:
		d.Claims.UnknownValue = append(d.Claims.UnknownValue, *c)
	case *TimeClaim:
		d.Claims.Time = append(d.Claims.Time, *c)
	case *TimeRangeClaim:
		d.Claims.TimeRange = append(d.Claims.TimeRange, *c)
	default:
		return errors.Errorf(`claim of type %T is not supported`, claim)
	}
	return nil
}

func (d *D) Size() int {
	return d.Claims.Size()
}

func (d *D) AllClaims() []Claim {
	v := AllClaimsVisitor{
		Result: []Claim{},
	}
	_ = d.Visit(&v)
	return v.Result
}

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
	d.Score = es.LowConfidence
	d.Scores = nil
	return nil
}
