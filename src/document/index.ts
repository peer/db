export type { Amount, Confidence, Reference, TimePrecision, Timestamp } from "@/document/types"

export {
  HighConfidence,
  HighNegationConfidence,
  LowConfidence,
  LowNegationConfidence,
  MediumConfidence,
  MediumNegationConfidence,
  NoConfidence,
} from "@/document/confidence"

export {
  AmountClaim,
  AmountIntervalClaim,
  CLAIM_TYPES_MAP,
  ClaimTypes,
  HTMLClaim,
  HasClaim,
  IdentifierClaim,
  LinkClaim,
  NoneClaim,
  ReferenceClaim,
  StringClaim,
  TimeClaim,
  TimeIntervalClaim,
  UNDETERMINED_LANGUAGE,
  UnknownClaim,
  getAllClaimsOfType,
  getAllClaimsOfTypeWithConfidence,
  getBestClaimOfType,
  getClaimsAndLanguageOfTypeWithConfidence,
  getClaimsListsOfType,
  getClaimsOfType,
  getClaimsOfTypeWithConfidence,
  selectClaimsByLanguage,
} from "@/document/claims"
export type { Claim, ClaimForType, ClaimTypeName, Claims, ClaimsContainer } from "@/document/claims"

export { D } from "@/document/document"

export {
  AddClaimChange,
  AmountClaimPatch,
  AmountIntervalClaimPatch,
  Changes,
  HTMLClaimPatch,
  HasClaimPatch,
  IdentifierClaimPatch,
  LinkClaimPatch,
  NoneClaimPatch,
  ReferenceClaimPatch,
  RemoveClaimChange,
  SetClaimChange,
  StringClaimPatch,
  TimeClaimPatch,
  TimeIntervalClaimPatch,
  UnknownClaimPatch,
  changeFrom,
  claimPatchFrom,
} from "@/document/patch"
export type { Change } from "@/document/patch"
