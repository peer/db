package peerdb

import (
	"net/http"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/waf"

	"gitlab.com/peerdb/peerdb/auth"
	internalSite "gitlab.com/peerdb/peerdb/internal/site"
)

// UserGetAPI is a GET HTTP request handler which returns what the site knows about one of its users:
// their profile fields and the roles they hold (see auth.Identity). The site's authenticator answers
// it, on behalf of the caller and with the caller's credentials, and remembers the answer for every
// user of the site, so what it tells one caller about a user it tells them all.
//
// Roles are what makes this useful beyond showing who somebody is: the permissions UI needs them to
// tell what a user can already do without the document granting it to them, which decides what
// granting them access actually has to add.
func (s *Service) UserGetAPI(w http.ResponseWriter, req *http.Request, params waf.Params) {
	ctx := req.Context()

	site := waf.MustGetSite[*internalSite.Site](ctx)

	if site.Authenticator == nil {
		s.NotFoundWithError(w, req, errors.New("site has no authenticator"))
		return
	}

	identity, errE := site.Authenticator.Identity(w, req, params["id"], site.Roles)
	if errors.Is(errE, auth.ErrIdentityNotFound) {
		s.NotFoundWithError(w, req, errE)
		return
	} else if errors.Is(errE, auth.ErrAccessDenied) {
		s.ForbiddenWithError(w, req, errE)
		return
	} else if errE != nil {
		s.InternalServerErrorWithError(w, req, errE)
		return
	}

	s.WriteJSON(w, req, identity, nil)
}
