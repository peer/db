package auth

var (
	TestingResolveAccessToken = resolveAccessToken //nolint:gochecknoglobals
	TestingWithSubject        = withSubject        //nolint:gochecknoglobals
	TestingWithRoles          = withRoles          //nolint:gochecknoglobals
)

const TestingAccessTokenCookieName = accessTokenCookieName
