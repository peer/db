package document

import (
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

type VisitResult int

const (
	Keep VisitResult = iota
	KeepAndStop
	Drop
	DropAndStop
)

type Visitor interface { //nolint:interfacebloat
	VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E)
	VisitReference(claim *ReferenceClaim) (VisitResult, errors.E)
	VisitText(claim *TextClaim) (VisitResult, errors.E)
	VisitString(claim *StringClaim) (VisitResult, errors.E)
	VisitAmount(claim *AmountClaim) (VisitResult, errors.E)
	VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E)
	VisitRelation(claim *RelationClaim) (VisitResult, errors.E)
	VisitFile(claim *FileClaim) (VisitResult, errors.E)
	VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E)
	VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E)
	VisitTime(claim *TimeClaim) (VisitResult, errors.E)
	VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E)
}

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
	for i := range c.Text {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitText(&c.Text[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.Text[k] = c.Text[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.Text) != k {
		c.Text = c.Text[:k]
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
	for i := range c.AmountRange {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitAmountRange(&c.AmountRange[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.AmountRange[k] = c.AmountRange[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.AmountRange) != k {
		c.AmountRange = c.AmountRange[:k]
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
	for i := range c.File {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitFile(&c.File[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.File[k] = c.File[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.File) != k {
		c.File = c.File[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.NoValue {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitNoValue(&c.NoValue[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.NoValue[k] = c.NoValue[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.NoValue) != k {
		c.NoValue = c.NoValue[:k]
	}
	if stopping {
		return nil
	}

	stopping = false
	k = 0
	for i := range c.UnknownValue {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitUnknownValue(&c.UnknownValue[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.UnknownValue[k] = c.UnknownValue[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.UnknownValue) != k {
		c.UnknownValue = c.UnknownValue[:k]
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
	for i := range c.TimeRange {
		var keep VisitResult
		if !stopping {
			keep, err = visitor.VisitTimeRange(&c.TimeRange[i])
			if err != nil {
				return err
			}
		}
		if stopping || keep == Keep || keep == KeepAndStop {
			if i != k {
				c.TimeRange[k] = c.TimeRange[i]
			}
			k++
		}
		if keep == KeepAndStop || keep == DropAndStop {
			stopping = true
		}
	}
	if len(c.TimeRange) != k {
		c.TimeRange = c.TimeRange[:k]
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

func (v *GetByIDVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitText(claim *TextClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitFile(claim *FileClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
	}
	return Keep, errE
}

func (v *GetByIDVisitor) VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E) {
	if claim.ID == v.ID {
		v.Result = claim
		return v.Action, nil
	}
	errE := claim.Visit(v)
	if v.Result != nil {
		return v.Action, errE
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

func (v *GetByPropIDVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitText(claim *TextClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitFile(claim *FileClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

func (v *GetByPropIDVisitor) VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E) {
	if claim.Prop.ID != nil && *claim.Prop.ID == v.ID {
		v.Result = append(v.Result, claim)
		return v.Action, nil
	}
	return Keep, nil
}

var _ Visitor = (*AllClaimsVisitor)(nil)

// AllClaimsVisitor returns all claims.
//
// AllClaimsVisitor does not recurse into meta claims.
type AllClaimsVisitor struct {
	Result []Claim
}

func (v *AllClaimsVisitor) VisitIdentifier(claim *IdentifierClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitReference(claim *ReferenceClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitText(claim *TextClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitString(claim *StringClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitAmount(claim *AmountClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitAmountRange(claim *AmountRangeClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitRelation(claim *RelationClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitFile(claim *FileClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitNoValue(claim *NoValueClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitUnknownValue(claim *UnknownValueClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitTime(claim *TimeClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}

func (v *AllClaimsVisitor) VisitTimeRange(claim *TimeRangeClaim) (VisitResult, errors.E) {
	v.Result = append(v.Result, claim)
	return Keep, nil
}
