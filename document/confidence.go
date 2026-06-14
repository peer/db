package document

import (
	"math"

	"gitlab.com/tozd/go/errors"
)

// Confidence represents the confidence level of a claim.
//
// Its range is [-1, 1] where negative value represents a
// confidence in a negation of the claim.
type Confidence float64

// validateConfidence checks that the confidence is inside its range.
func validateConfidence(confidence Confidence) errors.E {
	if math.IsInf(float64(confidence), 0) || math.IsNaN(float64(confidence)) || confidence < -1 || confidence > 1 {
		return errors.New("confidence out of range [-1, 1]")
	}
	return nil
}

const (
	// HighConfidence represents a high confidence score of 1.0.
	HighConfidence Confidence = 1.0
	// MediumConfidence represents a medium confidence score of 0.75.
	MediumConfidence Confidence = 0.75
	// LowConfidence represents a low confidence score of 0.5.
	LowConfidence Confidence = 0.5
	// NoConfidence represents no confidence with a score of 0.0.
	NoConfidence Confidence = 0.0
	// HighNegationConfidence represents high confidence in a negation with a score of -1.0.
	HighNegationConfidence Confidence = -1.0
	// MediumNegationConfidence represents medium confidence in a negation with a score of -0.75.
	MediumNegationConfidence Confidence = -0.75
	// LowNegationConfidence represents low confidence in a negation with a score of -0.5.
	LowNegationConfidence Confidence = -0.5
)
