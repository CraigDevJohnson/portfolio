package partials

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestFooterNavigationGroupsAreExplicit(t *testing.T) {
	groups := footerNavGroups()
	if len(groups) != 2 {
		t.Fatalf("footerNavGroups() returned %d groups, want 2", len(groups))
	}
	if groups[0].Label != "Portfolio" || groups[1].Label != "Tools" {
		t.Fatalf("footer groups = %q, %q; want Portfolio, Tools", groups[0].Label, groups[1].Label)
	}

	html := renderComponent(t, Footer(""))

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

func TestFooterNavigationIdentifiesCurrentDestination(t *testing.T) {
	for _, item := range navItems() {
		t.Run(item.Page, func(t *testing.T) {
			markup := renderComponent(t, Footer(item.Page))
			links := footerTestElements(t, markup, "a")
			activeCount := 0
			for _, link := range links {
				if footerTestAttribute(link, "aria-current") != "page" {
					continue
				}
				activeCount++
				if href := footerTestAttribute(link, "href"); href != item.Href {
					t.Errorf("current footer destination = %q, want %q", href, item.Href)
				}
			}
			if activeCount != 1 {
				t.Errorf("current footer destination count = %d, want 1", activeCount)
			}
		})
	}
	if markup := renderComponent(t, Footer("portal")); strings.Contains(markup, `aria-current="page"`) {
		t.Error("footer marks a public destination current for an unknown route")
	}
}

func TestFooterConnectUsesNamedDecorativeIcons(t *testing.T) {
	links := footerTestElements(t, renderComponent(t, Footer("home")), "a")
	for _, expected := range []struct {
		href     string
		label    string
		icon     UIIconName
		external bool
	}{
		{href: "https://github.com/CraigDevJohnson", label: "GitHub", icon: UIIconGitHub, external: true},
		{href: "https://linkedin.com/in/craigdevjohnson", label: "LinkedIn", icon: UIIconLinkedIn, external: true},
		{href: "mailto:opportunity@craigdevjohnson.com", label: "Email", icon: UIIconMail},
	} {
		t.Run(expected.label, func(t *testing.T) {
			link := footerTestLink(t, links, expected.href)
			var icon *html.Node
			for child := range link.Descendants() {
				if child.Type == html.ElementNode && child.Data == "svg" {
					icon = child
					break
				}
			}
			if icon == nil {
				t.Fatal("footer connection has no SVG icon")
			}
			if footerTestAttribute(icon, "data-ui-icon") != string(expected.icon) || footerTestAttribute(icon, "aria-hidden") != "true" || footerTestAttribute(icon, "focusable") != "false" {
				t.Error("footer connection icon is incorrect or is exposed to assistive technology")
			}
			wantText := expected.label
			if expected.external {
				wantText += " (opens in a new tab)"
				if footerTestAttribute(link, "target") != "_blank" || footerTestAttribute(link, "rel") != "noopener noreferrer" {
					t.Error("external footer connection lacks consistent new-tab attributes")
				}
			} else if footerTestAttribute(link, "target") != "" {
				t.Error("email connection should use the user's mail handler without forcing a new tab")
			}
			if got := footerTestText(link); got != wantText {
				t.Errorf("footer connection text = %q, want %q", got, wantText)
			}
		})
	}
}

func TestFooterStackUsesCanonicalBrandsAndReadableDescriptions(t *testing.T) {
	markup := renderComponent(t, Footer("home"))
	links := footerTestElements(t, markup, "a")
	for _, expected := range []struct {
		href string
		text string
	}{
		{href: "https://go.dev/", text: "Go programming language"},
		{href: "https://templ.guide/", text: "templ components"},
		{href: "https://htmx.org/", text: "htmx library"},
		{href: "https://tailwindcss.com/", text: "Tailwind CSS framework"},
		{href: "https://aws.amazon.com/lambda/", text: "AWS Lambda hosting"},
		{href: "https://opentofu.org/", text: "OpenTofu infrastructure as code"},
		{href: "https://github.com/features/actions", text: "GitHub Actions automation"},
	} {
		t.Run(expected.text, func(t *testing.T) {
			link := footerTestLink(t, links, expected.href)
			if got, want := footerTestText(link), expected.text+" (opens in a new tab)"; got != want {
				t.Errorf("footer stack text = %q, want %q", got, want)
			}
			if footerTestAttribute(link, "target") != "_blank" || footerTestAttribute(link, "rel") != "noopener noreferrer" {
				t.Error("footer stack link lacks consistent new-tab attributes")
			}
		})
	}
	if !strings.Contains(markup, ">AI-assisted development</p>") || strings.Contains(markup, "Copilot") {
		t.Error("footer must credit AI-assisted development without naming one AI vendor")
	}
}

func footerTestElements(t *testing.T, markup, name string) []*html.Node {
	t.Helper()
	document, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		t.Fatal(err)
	}
	var elements []*html.Node
	for node := range document.Descendants() {
		if node.Type == html.ElementNode && node.Data == name {
			elements = append(elements, node)
		}
	}
	return elements
}

func footerTestLink(t *testing.T, links []*html.Node, href string) *html.Node {
	t.Helper()
	for _, link := range links {
		if footerTestAttribute(link, "href") == href {
			return link
		}
	}
	t.Fatalf("footer has no link to %q", href)
	return nil
}

func footerTestAttribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func footerTestText(node *html.Node) string {
	var text strings.Builder
	for descendant := range node.Descendants() {
		if descendant.Type == html.TextNode {
			text.WriteString(descendant.Data)
		}
	}
	return strings.Join(strings.Fields(text.String()), " ")
}
