package document

import "gitlab.com/tozd/identifier"

//nolint:gochecknoglobals
var (
	TestingGetFallbackLanguages = getFallbackLanguages
	TestingIsRecognizedLanguage = isRecognizedLanguage
)

func TestingGetClaimsOfType[T any, PT interface {
	*T
	Claim
}](claims Claims, propID identifier.Identifier) []PT {
	return getClaimsOfType[T, PT](claims, propID)
}

func TestingGetAllClaimsOfType[T any, PT interface {
	*T
	Claim
}](claims Claims) []PT {
	return getAllClaimsOfType[T, PT](claims)
}
