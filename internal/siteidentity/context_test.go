package siteidentity

import (
	"context"
	"testing"
)

func TestSoccerAccessFailsClosedWithoutACurrentGrantedOwner(t *testing.T) {
	owner := &Principal{Issuer: "https://issuer.example.com/pool", Subject: "owner-subject"}
	for _, tc := range []struct {
		name                string
		ctx                 context.Context
		private, ownerState bool
	}{
		{name: "no site identity attached", ctx: context.Background()},
		{name: "signed out", ctx: WithRequestIdentity(context.Background(), nil, nil, "/soccer")},
		{name: "signed in without the soccer grant", ctx: WithRequestIdentity(context.Background(), owner, []Grant{GrantManagement}, "/soccer")},
		{name: "owner holding the soccer grant", ctx: WithRequestIdentity(context.Background(), owner, []Grant{GrantSoccer}, "/soccer"), private: true, ownerState: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := SoccerPrivateAllowed(tc.ctx); got != tc.private {
				t.Errorf("SoccerPrivateAllowed = %t, want %t", got, tc.private)
			}
			if got := SoccerOwnerAllowed(tc.ctx, owner.Issuer, owner.Subject); got != tc.ownerState {
				t.Errorf("SoccerOwnerAllowed(owner) = %t, want %t", got, tc.ownerState)
			}
			if SoccerOwnerAllowed(tc.ctx, "", "") {
				t.Error("ownerless private Soccer state was allowed")
			}
			if SoccerOwnerAllowed(tc.ctx, owner.Issuer, "another-subject") {
				t.Error("another subject's private Soccer state was allowed")
			}
		})
	}
}
