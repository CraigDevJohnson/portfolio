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
			if !navigateHTMX(w, r, "/sign-in?return_to=%2Fmgmt") {
				http.Redirect(w, r, "/sign-in?return_to=%2Fmgmt", http.StatusSeeOther)
			}
			return
		}
		if !siteidentity.HasGrant(r.Context(), siteidentity.GrantManagement) {
			// An HTMX fragment request opens the full denial page instead of swapping it in.
			if !navigateHTMX(w, r, "/mgmt") {
				h.renderErrorPage(w, r, http.StatusForbidden, managementAccessDenied)
			}
			return
		}
		next(w, r)
	}
}

// navigateHTMX sends an HTMX request to target as a full page load, and
// reports false for an ordinary request so the caller can respond normally.
func navigateHTMX(w http.ResponseWriter, r *http.Request, target string) bool {
	if r.Header.Get("HX-Request") != "true" {
		return false
	}
	w.Header().Set("HX-Redirect", target)
	w.WriteHeader(http.StatusNoContent)
	return true
}
