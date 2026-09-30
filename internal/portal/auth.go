package portal

import (
	"net/http"

	"portfolio/internal/siteidentity"
)

// managementAccessDenied explains a signed-in account without the management grant.
const managementAccessDenied = "Your account does not have management access."

// RequireManagement protects every live portal route using the current site grant.
func (h *Handler) RequireManagement(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, signedIn := siteidentity.PrincipalFromContext(r.Context()); !signedIn {
			target := "/sign-in?return_to=%2Fmgmt"
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", target)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
		if !siteidentity.HasGrant(r.Context(), siteidentity.GrantManagement) {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/mgmt")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			h.renderErrorPage(w, r, http.StatusForbidden, managementAccessDenied)
			return
		}
		next(w, r)
	}
}
