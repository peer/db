package document

import (
	"cmp"
	"slices"

	"gitlab.com/tozd/identifier"

	internalCore "gitlab.com/peerdb/peerdb/internal/core"
)

// InstanceOf returns IDs of the classes the document is an instance of, deduped: the targets of the
// document's instance of claims with at least LowConfidence, sorted by decreasing confidence of the
// claims naming them, with ties sorted ascending by ID. A class named by several claims ranks by its
// highest confidence. It returns nil when there are none.
func (d *D) InstanceOf() []identifier.Identifier {
	claims := GetClaimsOfTypeWithConfidence[ReferenceClaim](d, internalCore.InstanceOfPropID, LowConfidence)
	if len(claims) == 0 {
		return nil
	}
	slices.SortFunc(claims, func(a, b *ReferenceClaim) int {
		if c := cmp.Compare(b.GetConfidence(), a.GetConfidence()); c != 0 {
			return c
		}
		return internalCore.CompareIdentifiers(a.To.ID, b.To.ID)
	})
	ids := make([]identifier.Identifier, 0, len(claims))
	for _, claim := range claims {
		if !slices.Contains(ids, claim.To.ID) {
			ids = append(ids, claim.To.ID)
		}
	}
	return ids
}
