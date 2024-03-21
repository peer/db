package peerdb

import (
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

type Document struct {
	document.CoreDocument

	Mnemonic document.Mnemonic    `json:"mnemonic,omitempty"`
	Claims   *document.ClaimTypes `json:"claims,omitempty"`
}

func (d Document) Reference() document.Reference {
	return document.Reference{
		ID:    &d.ID,
		Score: d.Score,
	}
}

func (d *Document) Visit(visitor document.Visitor) errors.E {
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

func (d *Document) Get(propID identifier.Identifier) []document.Claim {
	v := document.GetByPropIDVisitor{
		ID:     propID,
		Action: document.Keep,
		Result: []document.Claim{},
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *Document) Remove(propID identifier.Identifier) []document.Claim {
	v := document.GetByPropIDVisitor{
		ID:     propID,
		Action: document.Drop,
		Result: []document.Claim{},
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *Document) GetByID(id identifier.Identifier) document.Claim { //nolint:ireturn
	v := document.GetByIDVisitor{
		ID:     id,
		Action: document.KeepAndStop,
		Result: nil,
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *Document) RemoveByID(id identifier.Identifier) document.Claim { //nolint:ireturn
	v := document.GetByIDVisitor{
		ID:     id,
		Action: document.DropAndStop,
		Result: nil,
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *Document) Add(claim document.Claim) errors.E {
	if claimID := claim.GetID(); d.GetByID(claimID) != nil {
		return errors.Errorf(`claim with ID "%s" already exists`, claimID)
	}
	if d.Claims == nil {
		d.Claims = &document.ClaimTypes{}
	}
	switch c := claim.(type) {
	case *document.IdentifierClaim:
		d.Claims.Identifier = append(d.Claims.Identifier, *c)
	case *document.ReferenceClaim:
		d.Claims.Reference = append(d.Claims.Reference, *c)
	case *document.TextClaim:
		d.Claims.Text = append(d.Claims.Text, *c)
	case *document.StringClaim:
		d.Claims.String = append(d.Claims.String, *c)
	case *document.AmountClaim:
		d.Claims.Amount = append(d.Claims.Amount, *c)
	case *document.AmountRangeClaim:
		d.Claims.AmountRange = append(d.Claims.AmountRange, *c)
	case *document.RelationClaim:
		d.Claims.Relation = append(d.Claims.Relation, *c)
	case *document.FileClaim:
		d.Claims.File = append(d.Claims.File, *c)
	case *document.NoValueClaim:
		d.Claims.NoValue = append(d.Claims.NoValue, *c)
	case *document.UnknownValueClaim:
		d.Claims.UnknownValue = append(d.Claims.UnknownValue, *c)
	case *document.TimeClaim:
		d.Claims.Time = append(d.Claims.Time, *c)
	case *document.TimeRangeClaim:
		d.Claims.TimeRange = append(d.Claims.TimeRange, *c)
	default:
		return errors.Errorf(`claim of type %T is not supported`, claim)
	}
	return nil
}

func (d *Document) AllClaims() []document.Claim {
	v := document.AllClaimsVisitor{
		Result: []document.Claim{},
	}
	_ = d.Visit(&v)
	return v.Result
}

func (d *Document) MergeFrom(other ...*Document) errors.E {
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
	d.Score = 0.5
	d.Scores = nil
	return nil
}
