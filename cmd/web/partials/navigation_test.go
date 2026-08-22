package partials

import (
	"strings"
	"testing"
)

func TestFooterNavigationGroupsAreExplicit(t *testing.T) {
	groups := footerNavGroups()
	if len(groups) != 2 {
		t.Fatalf("footerNavGroups() returned %d groups, want 2", len(groups))
	}
	if groups[0].Label != "Portfolio" || groups[1].Label != "Tools" {
		t.Fatalf("footer groups = %q, %q; want Portfolio, Tools", groups[0].Label, groups[1].Label)
	}

	html := renderComponent(t, Footer())

	portfolioHeading := `<h3 class="footer-heading mb-2 text-sm uppercase tracking-[0.05em] text-copy-muted">Portfolio</h3>`
	toolsHeading := `<h3 class="footer-heading mb-2 text-sm uppercase tracking-[0.05em] text-copy-muted">Tools</h3>`
	if !strings.Contains(html, portfolioHeading) || !strings.Contains(html, toolsHeading) {
		t.Fatalf("footer headings do not contain explicit Portfolio and Tools groups: %s", html)
	}

	portfolioStart := strings.Index(html, portfolioHeading)
	toolsStart := strings.Index(html, toolsHeading)
	connectStart := strings.Index(html, ">Connect</h3>")
	if portfolioStart < 0 || toolsStart <= portfolioStart || connectStart <= toolsStart {
		t.Fatalf("footer group order is not Portfolio, Tools, Connect: %s", html)
	}

	portfolioHTML := html[portfolioStart:toolsStart]
	toolsHTML := html[toolsStart:connectStart]
	for _, label := range []string{"Home", "About", "Experience", "Skills", "Projects", "Education", "Contact"} {
		if !strings.Contains(portfolioHTML, ">"+label+"</a>") {
			t.Errorf("Portfolio footer group does not contain %q: %s", label, portfolioHTML)
		}
	}
	if !strings.Contains(toolsHTML, ">Soccer</a>") {
		t.Errorf("Tools footer group does not contain Soccer: %s", toolsHTML)
	}
}

func TestNavigationRendersExactlyOneActiveItem(t *testing.T) {
	for _, item := range navItems() {
		t.Run(item.Page, func(t *testing.T) {
			html := renderComponent(t, NavLinks(item.Page))
			if got := strings.Count(html, `aria-current="page"`); got != 1 {
				t.Fatalf("NavLinks(%q) aria-current count = %d, want 1: %s", item.Page, got, html)
			}
			if !strings.Contains(html, `data-nav-page="`+item.Page+`" aria-current="page"`) {
				t.Errorf("NavLinks(%q) does not mark the matching destination active: %s", item.Page, html)
			}
		})
	}
}
