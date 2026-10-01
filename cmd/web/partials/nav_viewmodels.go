package partials

import (
	"context"
	"net/url"

	"portfolio/internal/siteidentity"
)

// AccountNavPropsFromContext presents current site identity in shared navigation.
func AccountNavPropsFromContext(ctx context.Context) AccountNavProps {
	return AccountNavProps{
		Email:           siteidentity.EmailForNavigation(ctx),
		ReturnTo:        siteidentity.ReturnToForNavigation(ctx),
		SignInAvailable: siteidentity.SignInAvailable(ctx),
	}
}

func signInHref(returnTo string) string {
	return "/sign-in?return_to=" + url.QueryEscape(returnTo)
}
