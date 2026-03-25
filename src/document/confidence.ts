import type { Confidence } from "@/document/types"

// HighConfidence represents a high confidence score of 1.0.
export const HighConfidence: Confidence = 1.0

// MediumConfidence represents a medium confidence score of 0.75.
export const MediumConfidence: Confidence = 0.75

// LowConfidence represents a low confidence score of 0.5.
export const LowConfidence: Confidence = 0.5

// NoConfidence represents no confidence with a score of 0.0.
export const NoConfidence: Confidence = 0.0

// HighNegationConfidence represents high confidence in a negation with a score of -1.0.
export const HighNegationConfidence: Confidence = -1.0

// MediumNegationConfidence represents medium confidence in a negation with a score of -0.75.
export const MediumNegationConfidence: Confidence = -0.75

// MediumNegationConfidence represents medium confidence in a negation with a score of -0.75.
export const LowNegationConfidence: Confidence = -0.5
