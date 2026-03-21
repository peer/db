package document

import (
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// VisitResult represents the result of visiting a claim during traversal.
type VisitResult int

const (
	// Keep indicates the claim should be kept and traversal should continue.
	Keep VisitResult = iota
	// KeepAndStop indicates the claim should be kept and traversal should stop.
	KeepAndStop
	// Drop indicates the claim should be dropped and traversal should continue.
	Drop
	// DropAndStop indicates the claim should be dropped and traversal should stop.
	DropAndStop
)

// Visitor is an interface for visiting different claim types in a document.
type Visitor interface { //nolint:interfacebloat
	VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E)
	VisitString(claim *StringClaim) (VisitResult, errors.E)
	VisitHTML(claim *HTMLClaim) (VisitResult, errors.E)
	VisitAmount(claim *AmountClaim) (VisitResult, errors.E)
	VisitAmountInterval(claim *AmountIntervalClaim) (VisitResult, errors.E)
	VisitTime(claim *TimeClaim) (VisitResult, errors.E)
	VisitTimeInterval(claim *TimeIntervalClaim) (VisitResult, errors.E)
	VisitReference(claim *ReferenceClaim) (VisitResult, errors.E)
	VisitRelation(claim *RelationClaim) (VisitResult, errors.E)
	VisitHas(claim *HasClaim) (VisitResult, errors.E)
	VisitNone(claim *NoneClaim) (VisitResult, errors.E)
	VisitUnknown(claim *UnknownClaim) (VisitResult, errors.E)
}

// Visit traverses all claims in the ClaimTypes and applies the visitor to each one.
func (c *ClaimTypes) Visit(visitor Visitor) errors.E { //nolint:maintidx
	if c == nil {
		return nil
	}

	var err errors.E

	stopping := false
	k := 0
	for i := range c.Identifier {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitIdentifier(&c.Identifier[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Identifier[k] = c.Identifier[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Identifier) != k {
		c.Identifier = c.Identifier[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.String {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitString(&c.String[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.String[k] = c.String[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.String) != k {
		c.String = c.String[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.HTML {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitHTML(&c.HTML[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.HTML[k] = c.HTML[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.HTML) != k {
		c.HTML = c.HTML[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Amount {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitAmount(&c.Amount[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Amount[k] = c.Amount[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Amount) != k {
		c.Amount = c.Amount[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.AmountInterval {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitAmountInterval(&c.AmountInterval[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.AmountInterval[k] = c.AmountInterval[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.AmountInterval) != k {
		c.AmountInterval = c.AmountInterval[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Time {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitTime(&c.Time[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Time[k] = c.Time[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Time) != k {
		c.Time = c.Time[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.TimeInterval {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitTimeInterval(&c.TimeInterval[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.TimeInterval[k] = c.TimeInterval[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.TimeInterval) != k {
		c.TimeInterval = c.TimeInterval[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Reference {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitReference(&c.Reference[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Reference[k] = c.Reference[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Reference) != k {
		c.Reference = c.Reference[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Relation {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitRelation(&c.Relation[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Relation[k] = c.Relation[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Relation) != k {
		c.Relation = c.Relation[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Has {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitHas(&c.Has[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Has[k] = c.Has[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Has) != k {
		c.Has = c.Has[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.None {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitNone(&c.None[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.None[k] = c.None[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.None) != k {
		c.None = c.None[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.Unknown {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitUnknown(&c.Unknown[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Unknown[k] = c.Unknown[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Unknown) != k {
		c.Unknown = c.Unknown[:k]
	}
	if stopping {
		return nil
	}

	return nil
}

// GetByIDVisitor recurses into meta claims.
type GetByIDVisitor struct {
	ID     identifier.Identifier
	Action VisitResult
	Result Claim
}

var _ Visitor = (*GetByIDVisitor)(nil)

// VisitIdentifier visits an identifier claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitString visits a string claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitHTML visits an HTML claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitHTML(claim *HTMLClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitAmount visits an amount claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitAmountInterval visits an amount interval claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitAmountInterval(claim *AmountIntervalClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitTime visits a time claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitTimeInterval visits a time interval claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitTimeInterval(claim *TimeIntervalClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitReference visits a reference claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitRelation visits a relation claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitHas visits a has claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitHas(claim *HasClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitNone visits a none claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitNone(claim *NoneClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

// VisitUnknown visits an unknown claim and checks if its ID matches the target ID.
func (v *GetByIDVisitor) VisitUnknown(claim *UnknownClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return KeepAndStop, errE
	}
	return Keep, errE
}

var _ Visitor = (*GetByPropIDVisitor)(nil)

// GetByPropIDVisitor does not recurse into meta claims.
type GetByPropIDVisitor struct {
	ID     identifier.Identifier
	Action VisitResult
	Result []Claim
}

// VisitIdentifier visits an identifier claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitString visits a string claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitHTML visits an HTML claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitHTML(claim *HTMLClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitAmount visits an amount claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitAmountInterval visits an amount interval claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitAmountInterval(claim *AmountIntervalClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitTime visits a time claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitTimeInterval visits a time interval claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitTimeInterval(claim *TimeIntervalClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitReference visits a reference claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitRelation visits a relation claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitHas visits a has claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitHas(claim *HasClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitNone visits a none claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitNone(claim *NoneClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

// VisitUnknown visits an unknown claim and checks if its property ID matches the target.
func (v *GetByPropIDVisitor) VisitUnknown(claim *UnknownClaim) (VisitResult, errors.E) {
	if claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

var _ Visitor = (*AllClaimsVisitor)(nil)

// AllClaimsVisitor is a Visitor that drives the AllClaims iterator.
//
// AllClaimsVisitor does not recurse into meta claims.
type AllClaimsVisitor struct {
	Yield func(Claim) bool
}

func (v *AllClaimsVisitor) visit(claim Claim) (VisitResult, errors.E) {
	if v.Yield(claim) {
		return Keep, nil
	}
	return KeepAndStop, nil
}

// VisitIdentifier calls yield with the identifier claim.
func (v *AllClaimsVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitString calls yield with the string claim.
func (v *AllClaimsVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitHTML calls yield with the HTML claim.
func (v *AllClaimsVisitor) VisitHTML(claim *HTMLClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitAmount calls yield with the amount claim.
func (v *AllClaimsVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitAmountInterval calls yield with the amount interval claim.
func (v *AllClaimsVisitor) VisitAmountInterval(claim *AmountIntervalClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitTime calls yield with the time claim.
func (v *AllClaimsVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitTimeInterval calls yield with the time interval claim.
func (v *AllClaimsVisitor) VisitTimeInterval(claim *TimeIntervalClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitReference calls yield with the reference claim.
func (v *AllClaimsVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitRelation calls yield with the relation claim.
func (v *AllClaimsVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitHas calls yield with the has claim.
func (v *AllClaimsVisitor) VisitHas(claim *HasClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitNone calls yield with the none claim.
func (v *AllClaimsVisitor) VisitNone(claim *NoneClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitUnknown calls yield with the unknown claim.
func (v *AllClaimsVisitor) VisitUnknown(claim *UnknownClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

var _ Visitor = (*AllClaimsWithMetaVisitor)(nil)

// AllClaimsWithMetaVisitor is a Visitor that drives the AllClaimsWithMeta iterator.
//
// AllClaimsWithMetaVisitor recurses into meta claims.
type AllClaimsWithMetaVisitor struct {
	Yield   func(Claim) bool
	stopped bool
}

func (v *AllClaimsWithMetaVisitor) visit(claim Claim) (VisitResult, errors.E) {
	if v.stopped {
		return KeepAndStop, nil
	}
	if !v.Yield(claim) {
		v.stopped = true
		return KeepAndStop, nil
	}
	// Recurse into meta claims.
	errE := claim.Visit(v)
	if errE != nil {
		return Keep, errE
	}
	if v.stopped {
		return KeepAndStop, nil
	}
	return Keep, nil
}

// VisitIdentifier calls yield with the identifier claim.
func (v *AllClaimsWithMetaVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitString calls yield with the string claim.
func (v *AllClaimsWithMetaVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitHTML calls yield with the HTML claim.
func (v *AllClaimsWithMetaVisitor) VisitHTML(claim *HTMLClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitAmount calls yield with the amount claim.
func (v *AllClaimsWithMetaVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitAmountInterval calls yield with the amount interval claim.
func (v *AllClaimsWithMetaVisitor) VisitAmountInterval(claim *AmountIntervalClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitTime calls yield with the time claim.
func (v *AllClaimsWithMetaVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitTimeInterval calls yield with the time interval claim.
func (v *AllClaimsWithMetaVisitor) VisitTimeInterval(claim *TimeIntervalClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitReference calls yield with the reference claim.
func (v *AllClaimsWithMetaVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitRelation calls yield with the relation claim.
func (v *AllClaimsWithMetaVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitHas calls yield with the has claim.
func (v *AllClaimsWithMetaVisitor) VisitHas(claim *HasClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitNone calls yield with the none claim.
func (v *AllClaimsWithMetaVisitor) VisitNone(claim *NoneClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}

// VisitUnknown calls yield with the unknown claim.
func (v *AllClaimsWithMetaVisitor) VisitUnknown(claim *UnknownClaim) (VisitResult, errors.E) {
	return v.visit(claim)
}
