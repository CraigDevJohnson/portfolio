package app

import (
	"fmt"
	"html"
	"mime"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var renderedHTMLIDPattern = regexp.MustCompile(`\sid="([^"]+)"`)

func TestBuildMuxPublicRouteRenderingSmoke(t *testing.T) {
	app := newTestApp(t)
	// Preview mode uses representative in-memory data and never initializes
	// Cognito, EC2, CloudWatch, or CloudWatch Logs clients.
	mux, _ := buildMux(app, app.Logger, true)

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		status      int
		mediaType   string
		bodyMarker  string
		bodyClass   string
		pageMarker  string
		pageAssert  func(*testing.T, string, string)
		shell       string
		fragment    bool
		location    string
	}{
		{
			name:       "home page",
			path:       "/",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "<title>Craig Johnson - Cloud Engineer Principal</title>",
			bodyClass:  "site-body page-home",
			pageMarker: "page-kit-page home-page",
			pageAssert: assertHomeSystemsOverlook,
			shell:      "public",
		},
		{
			name:       "about page",
			path:       "/about",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "<title>About Me - Craig Johnson</title>",
			bodyClass:  "site-body page-about",
			pageMarker: "page-kit-page about-page",
			pageAssert: assertAboutAlaskaSwitchback,
			shell:      "public",
		},
		{
			name:       "experience page",
			path:       "/experience",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "<title>Experience - Craig Johnson</title>",
			bodyClass:  "site-body page-experience",
			pageMarker: "page-kit-page experience-page",
			pageAssert: assertExperienceCareerEras,
			shell:      "public",
		},
		{
			name:       "skills page",
			path:       "/skills",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "<title>Technical Proficiencies - Craig Johnson</title>",
			bodyClass:  "site-body page-skills",
			pageMarker: "page-kit-page skills-page",
			pageAssert: assertSkillsCardFieldTrail,
			shell:      "public",
		},
		{
			name:       "projects page",
			path:       "/projects",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "<title>Projects - Craig Johnson</title>",
			bodyClass:  "site-body page-projects",
			pageMarker: "page-kit-page projects-page",
			pageAssert: assertProjectsDossiers,
			shell:      "public",
		},
		{
			name:       "education page",
			path:       "/education",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "<title>Education &amp; Certifications - Craig Johnson</title>",
			bodyClass:  "site-body page-education",
			pageMarker: "page-kit-page education-page",
			pageAssert: assertEducationLearningFieldGuide,
			shell:      "public",
		},
		{
			name:       "contact page",
			path:       "/contact",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "<title>Get in Touch - Craig Johnson</title>",
			bodyClass:  "site-body page-contact",
			pageMarker: "page-kit-page contact-page",
			pageAssert: assertContactCorrespondenceWindow,
			shell:      "public",
		},
		{
			name:       "soccer page",
			path:       "/soccer",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "<title>Soccer Schedule Download - Craig Johnson</title>",
			bodyClass:  "site-body page-soccer soccer-theme",
			pageMarker: "page-kit-page soccer-page",
			shell:      "public",
		},
		{
			name:       "filtered skills fragment",
			path:       "/skills/filtered?category=Cloud+Platforms&proficiency=advanced",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "skills shown",
			fragment:   true,
		},
		{
			name:       "skill detail fragment",
			path:       "/skills/detail?id=2",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "skill-detail-name",
			fragment:   true,
		},
		{
			name:       "soccer session fragment",
			path:       "/soccer/session",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "Not imported",
			fragment:   true,
		},
		{
			name:        "soccer schedule result fragment",
			method:      http.MethodPost,
			path:        "/soccer/fetch",
			body:        "player_ids=invalid",
			contentType: "application/x-www-form-urlencoded",
			status:      http.StatusOK,
			mediaType:   "text/html",
			bodyMarker:  "selected players were invalid",
			fragment:    true,
		},
		{
			name:       "management metrics fragment",
			path:       "/mgmt/instances/i-0f1e2d3c4b5a69788/metrics",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "CPU %",
			fragment:   true,
		},
		{
			name:       "management logs fragment",
			path:       "/mgmt/instances/i-0f1e2d3c4b5a69788/logs",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "Health check passed",
			fragment:   true,
		},
		{
			name:       "management action result fragment",
			method:     http.MethodPost,
			path:       "/mgmt/instances/i-0f1e2d3c4b5a69788/restart",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: "no AWS restart action was sent",
			fragment:   true,
		},
		{
			name:       "local management preview",
			path:       "/mgmt",
			status:     http.StatusOK,
			mediaType:  "text/html",
			bodyMarker: `id="portal-console-title"`,
			bodyClass:  "site-body page-portal",
			pageMarker: "page-kit-page portal-page portal-console-page",
			pageAssert: assertPortalCardFieldTrail,
			shell:      "operator",
		},
		{
			name:       "home compatibility redirect",
			path:       "/home",
			status:     http.StatusMovedPermanently,
			mediaType:  "text/html",
			bodyMarker: "Moved Permanently",
			location:   "/",
		},
		{
			name:       "obsolete skills grid fragment",
			path:       "/skills/grid",
			status:     http.StatusNotFound,
			mediaType:  "text/plain",
			bodyMarker: "404 page not found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method := test.method
			if method == "" {
				method = http.MethodGet
			}
			request := httptest.NewRequest(method, test.path, strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("%s %s status = %d, want %d", method, test.path, response.Code, test.status)
			}
			assertResponseMediaType(t, response, test.mediaType)
			body := response.Body.String()
			if !strings.Contains(body, test.bodyMarker) {
				t.Errorf("%s %s body does not contain %q", method, test.path, test.bodyMarker)
			}
			if test.bodyClass != "" {
				assertRenderedPageShell(t, test.path, body, test.bodyClass, test.pageMarker, test.shell)
			}
			if test.pageAssert != nil {
				test.pageAssert(t, test.path, body)
			}
			if test.fragment && strings.Contains(body, "<!DOCTYPE html>") {
				t.Errorf("%s %s fragment unexpectedly contains a full document", method, test.path)
			}
			if test.fragment {
				requireNoFragmentTrail(t, test.path, body)
			}
			if location := response.Header().Get("Location"); location != test.location {
				t.Errorf("GET %s Location = %q, want %q", test.path, location, test.location)
			}
		})
	}
}

func assertSkillsCardFieldTrail(t *testing.T, path, body string) {
	t.Helper()

	filterable, err := findTestHTMLElementMarkup(body, `id="skills-filterable"`)
	if err != nil {
		t.Errorf("GET %s Skills catalog boundaries: %v", path, err)
		return
	}
	assertRenderedMarkerOrder(t, path, filterable, []string{
		`class="signal-trail signal-trail-workbench skills-workbench-trail"`,
		`class="skills-results-heading"`,
		`class="skills-catalog"`,
	})
}

func assertContactCorrespondenceWindow(t *testing.T, path, body string) {
	t.Helper()

	assertOrderedPageRegions(t, path, body, "correspondence-window", []string{"intro", "availability", "channels", "expertise"})
	if count := strings.Count(body, `data-signal-trail`); count != 1 {
		t.Errorf("GET %s signal trail count = %d, want 1", path, count)
	}
	if !strings.Contains(body, `class="signal-trail signal-trail-correspondence contact-correspondence-trail"`) {
		t.Errorf("GET %s does not render the typed correspondence trail", path)
	}
	correspondence, err := findTestHTMLElementMarkup(body, `class="contact-correspondence-route"`)
	if err != nil {
		t.Errorf("GET %s correspondence route boundaries: %v", path, err)
	} else {
		assertRenderedMarkerOrder(t, path, correspondence, []string{
			`class="signal-trail signal-trail-correspondence contact-correspondence-trail"`,
			`data-region="availability"`,
			`data-region="channels"`,
		})
	}
	if count := strings.Count(body, `<h1`); count != 1 {
		t.Errorf("GET %s h1 count = %d, want 1", path, count)
	}
	for _, marker := range []string{
		`class="contact-availability-title`,
		`<h2 class="page-section-title">Choose the easiest way to reach me</h2>`,
		`<h2 class="page-section-title">Expertise that delivers</h2>`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("GET %s sequential Contact hierarchy missing %q", path, marker)
		}
	}
	for _, icon := range []string{"mail", "linkedin", "github", "architecture", "automation", "security", "observability"} {
		marker := `data-ui-icon="` + icon + `"`
		if count := strings.Count(body, marker); count != 1 {
			t.Errorf("GET %s Contact icon %q count = %d, want 1", path, icon, count)
		}
	}

	if count := strings.Count(body, `ui-action-primary`); count != 1 {
		t.Errorf("GET %s primary action count = %d, want sole email primary", path, count)
	}
	assertContactActionVariant(t, path, body, `href="mailto:opportunity@craigdevjohnson.com"`, "ui-action-primary", false)
	assertContactActionVariant(t, path, body, `href="https://linkedin.com/in/craigdevjohnson"`, "ui-action-secondary", true)
	assertContactActionVariant(t, path, body, `href="https://github.com/CraigDevJohnson"`, "ui-action-secondary", true)

	cta, err := findTestHTMLElementMarkup(body, `class="contact-cta-region`)
	if err != nil {
		t.Errorf("GET %s Contact CTA boundaries: %v", path, err)
		return
	}
	if strings.Contains(cta, "ui-action-primary") {
		t.Errorf("GET %s Projects/Experience CTA unexpectedly contains a primary action", path)
	}
}

func assertContactActionVariant(t *testing.T, path, body, hrefMarker, variant string, external bool) {
	t.Helper()

	openingTags, err := scanTestHTMLTagBoundaries(body)
	if err != nil {
		t.Errorf("GET %s parse Contact actions: %v", path, err)
		return
	}
	found := false
	for _, tag := range openingTags {
		if tag.closing || tag.name != "a" {
			continue
		}
		opening := body[tag.start:tag.end]
		if !strings.Contains(opening, hrefMarker) || !strings.Contains(opening, variant) {
			continue
		}
		if external && (!strings.Contains(opening, `target="_blank"`) || !strings.Contains(opening, `rel="noopener noreferrer"`)) {
			t.Errorf("GET %s action %s lacks external-link semantics: %s", path, hrefMarker, opening)
		}
		found = true
	}
	if !found {
		t.Errorf("GET %s does not render %s with typed variant %q", path, hrefMarker, variant)
	}
}

func assertEducationLearningFieldGuide(t *testing.T, path, body string) {
	t.Helper()

	assertOrderedPageRegions(t, path, body, "learning-field-guide", []string{"intro", "stats", "foundation", "domains", "cta"})
	if count := strings.Count(body, `data-signal-trail`); count != 1 {
		t.Errorf("GET %s signal trail count = %d, want 1", path, count)
	}
	if !strings.Contains(body, `class="signal-trail signal-trail-field-guide education-field-guide-trail"`) {
		t.Errorf("GET %s does not render the typed field-guide trail", path)
	}
	if count := strings.Count(body, `<h1`); count != 1 {
		t.Errorf("GET %s h1 count = %d, want 1", path, count)
	}
	foundationHeading, foundationErr := findTestHTMLElementOpeningTag(body, `class="education-foundation-heading`)
	degreeHeading, degreeErr := findTestHTMLElementOpeningTag(body, `class="education-degree-title"`)
	if foundationErr != nil || foundationHeading.name != "h2" || degreeErr != nil || degreeHeading.name != "h3" {
		t.Errorf("GET %s foundation hierarchy = h2(%v, %v), degree h3(%v, %v)", path, foundationHeading.name, foundationErr, degreeHeading.name, degreeErr)
	}
	foundation, foundationMarkupErr := findTestHTMLElementMarkup(body, `data-region="foundation"`)
	if foundationMarkupErr != nil {
		t.Errorf("GET %s foundation boundaries: %v", path, foundationMarkupErr)
	} else {
		for _, marker := range []string{
			`data-degree-card`,
			`data-degree-image-fallback`,
			`class="education-degree-logo"`,
			`data-remote-image`,
			`alt=""`,
		} {
			if !strings.Contains(foundation, marker) {
				t.Errorf("GET %s degree image-failure contract missing %q", path, marker)
			}
		}
	}

	domains, err := findTestHTMLElementMarkup(body, `data-region="domains"`)
	if err != nil {
		t.Errorf("GET %s credential domains: %v", path, err)
		return
	}
	assertRenderedMarkerOrder(t, path, domains, []string{
		`education-domains-intro`,
		`class="signal-trail signal-trail-field-guide education-field-guide-trail"`,
		`class="education-domain-mosaic"`,
	})
	wantDomains := []struct {
		id    string
		count int
	}{
		{id: "cloud", count: 3},
		{id: "microsoft", count: 2},
		{id: "linux", count: 1},
		{id: "security", count: 1},
		{id: "delivery", count: 3},
	}
	for _, want := range wantDomains {
		marker := `data-domain="` + want.id + `"`
		if count := strings.Count(domains, marker); count != 1 {
			t.Errorf("GET %s domain %q count = %d, want 1", path, want.id, count)
			continue
		}
		domain, domainErr := findTestHTMLElementMarkup(domains, marker)
		if domainErr != nil {
			t.Errorf("GET %s domain %q boundaries: %v", path, want.id, domainErr)
			continue
		}
		if count := strings.Count(domain, `data-credential-card`); count != want.count {
			t.Errorf("GET %s domain %q credential count = %d, want %d", path, want.id, count, want.count)
		}
		domainHeading, domainHeadingErr := findTestHTMLElementOpeningTag(domain, `class="education-domain-title"`)
		credentialHeadingCount := strings.Count(domain, `<h4 class="education-credential-title">`)
		if domainHeadingErr != nil || domainHeading.name != "h3" || credentialHeadingCount != want.count {
			t.Errorf("GET %s domain %q hierarchy = h3(%v, %v), credential h4 count %d want %d", path, want.id, domainHeading.name, domainHeadingErr, credentialHeadingCount, want.count)
		}
	}
	if count := strings.Count(domains, `data-credential-card`); count != 10 {
		t.Errorf("GET %s total credential card count = %d, want 10", path, count)
	}
	if count := strings.Count(domains, `loading="eager"`); count != 1 {
		t.Errorf("GET %s eager credential image count = %d, want 1", path, count)
	}
	if count := strings.Count(domains, `loading="lazy"`); count != 9 {
		t.Errorf("GET %s lazy credential image count = %d, want 9", path, count)
	}
}

func assertProjectsDossiers(t *testing.T, path, body string) {
	t.Helper()

	assertOrderedPageRegions(t, path, body, "project-dossiers", []string{"intro", "lead", "support", "cta"})
	if count := strings.Count(body, `data-signal-trail`); count != 1 {
		t.Errorf("GET %s signal trail count = %d, want 1", path, count)
	}
	if count := strings.Count(body, `class="project-dossier project-dossier-lead`); count != 1 {
		t.Errorf("GET %s lead dossier count = %d, want 1", path, count)
	}
	if count := strings.Count(body, `class="project-dossier project-dossier-support`); count != 2 {
		t.Errorf("GET %s supporting dossier count = %d, want 2", path, count)
	}
	composition, err := findTestHTMLElementMarkup(body, `class="project-dossier-composition"`)
	if err != nil {
		t.Errorf("GET %s dossier composition boundaries: %v", path, err)
	} else {
		assertRenderedMarkerOrder(t, path, composition, []string{
			`class="signal-trail signal-trail-dossier projects-dossier-signal"`,
			`data-region="lead"`,
			`data-region="support"`,
		})
	}
	for _, marker := range []string{
		`data-image-ratio="portrait"`,
		`data-image-ratio="landscape"`,
		`data-image-ratio="square"`,
		`>Problem<`,
		`>Approach<`,
		`>Outcome<`,
		`>Technology<`,
		`>Destination<`,
		`href="https://craigdevjohnson.com"`,
		`href="https://github.com/CraigDevJohnson/portfolio"`,
		`href="/soccer"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("GET %s project dossiers missing %q", path, marker)
		}
	}
	soccerLink := regexp.MustCompile(`<a[^>]+href="/soccer"[^>]*>`).FindString(body)
	if soccerLink == "" {
		t.Errorf("GET %s does not render a Soccer destination anchor", path)
	} else if strings.Contains(soccerLink, `target="_blank"`) || strings.Contains(soccerLink, `rel="noopener noreferrer"`) {
		t.Errorf("GET %s internal Soccer destination has external-link semantics: %s", path, soccerLink)
	}
}

func assertPortalCardFieldTrail(t *testing.T, path, body string) {
	t.Helper()

	workspace, err := findTestHTMLElementMarkup(body, `class="page-kit-page portal-page portal-console-page"`)
	if err != nil {
		t.Errorf("GET %s operator workspace boundaries: %v", path, err)
		return
	}
	assertRenderedMarkerOrder(t, path, workspace, []string{
		`class="signal-trail signal-trail-operator portal-operator-trail"`,
		`class="portal-operator-header"`,
		`class="page-section-tight portal-console-main portal-instances-section"`,
	})
}

func assertRenderedMarkerOrder(t *testing.T, path, markup string, markers []string) {
	t.Helper()

	previous := -1
	for _, marker := range markers {
		index := strings.Index(markup, marker)
		if index < 0 {
			t.Errorf("GET %s card field does not contain %q", path, marker)
			continue
		}
		if index <= previous {
			t.Errorf("GET %s card field marker %q is not after its predecessor", path, marker)
		}
		previous = index
	}
}

func assertOrderedPageRegions(t *testing.T, path, body, layout string, regions []string) {
	t.Helper()

	if err := validateOrderedPageRegions(body, layout, regions); err != nil {
		t.Errorf("GET %s ordered regions: %v", path, err)
	}
}

func validateOrderedPageRegions(body, layout string, regions []string) error {
	if marker := `data-layout="` + layout + `"`; !strings.Contains(body, marker) {
		return fmt.Errorf("body does not contain layout marker %q", marker)
	}

	if count := strings.Count(body, `data-region="`); count != len(regions) {
		return fmt.Errorf("data-region marker count = %d, want %d", count, len(regions))
	}

	previousIndex := -1
	for _, region := range regions {
		marker := `data-region="` + region + `"`
		if count := strings.Count(body, marker); count != 1 {
			return fmt.Errorf("region marker %q count = %d, want 1", marker, count)
		}
		index := strings.Index(body, marker)
		if index <= previousIndex {
			return fmt.Errorf("region marker %q is out of order", marker)
		}
		previousIndex = index
	}

	return nil
}

type testHTMLTagBoundary struct {
	start       int
	end         int
	name        string
	closing     bool
	selfClosing bool
}

type testHTMLElementBounds struct {
	start int
	end   int
}

func findTestHTMLElementMarkup(body, openingMarker string) (string, error) {
	bounds, err := findTestHTMLElementBounds(body, openingMarker)
	if err != nil {
		return "", err
	}
	return body[bounds.start:bounds.end], nil
}

func findTestHTMLElementBounds(body, openingMarker string) (testHTMLElementBounds, error) {
	tags, err := scanTestHTMLTagBoundaries(body)
	if err != nil {
		return testHTMLElementBounds{}, err
	}

	openingIndex := -1
	for index, tag := range tags {
		if tag.closing || !strings.Contains(body[tag.start:tag.end], openingMarker) {
			continue
		}
		if openingIndex != -1 {
			return testHTMLElementBounds{}, fmt.Errorf("multiple opening tags contain marker %q", openingMarker)
		}
		openingIndex = index
	}
	if openingIndex == -1 {
		return testHTMLElementBounds{}, fmt.Errorf("no opening tag contains marker %q", openingMarker)
	}

	opening := tags[openingIndex]
	if opening.selfClosing {
		return testHTMLElementBounds{}, fmt.Errorf("element containing marker %q is self-closing", openingMarker)
	}

	depth := 1
	for _, tag := range tags[openingIndex+1:] {
		if tag.name != opening.name {
			continue
		}
		if tag.closing {
			depth--
			if depth == 0 {
				return testHTMLElementBounds{start: opening.start, end: tag.end}, nil
			}
			continue
		}
		if !tag.selfClosing {
			depth++
		}
	}

	return testHTMLElementBounds{}, fmt.Errorf("element <%s> containing marker %q is not closed", opening.name, openingMarker)
}

func findTestHTMLElementOpeningTag(body, openingMarker string) (testHTMLTagBoundary, error) {
	tags, err := scanTestHTMLTagBoundaries(body)
	if err != nil {
		return testHTMLTagBoundary{}, err
	}
	matchIndex := -1
	for index, tag := range tags {
		if tag.closing || !strings.Contains(body[tag.start:tag.end], openingMarker) {
			continue
		}
		if matchIndex != -1 {
			return testHTMLTagBoundary{}, fmt.Errorf("multiple opening tags contain marker %q", openingMarker)
		}
		matchIndex = index
	}
	if matchIndex == -1 {
		return testHTMLTagBoundary{}, fmt.Errorf("no opening tag contains marker %q", openingMarker)
	}
	return tags[matchIndex], nil
}

func findTestHTMLDirectChildOpenings(body, parentMarker string) ([]testHTMLTagBoundary, error) {
	tags, err := scanTestHTMLTagBoundaries(body)
	if err != nil {
		return nil, err
	}
	parentIndex := -1
	for index, tag := range tags {
		if tag.closing || !strings.Contains(body[tag.start:tag.end], parentMarker) {
			continue
		}
		if parentIndex != -1 {
			return nil, fmt.Errorf("multiple opening tags contain parent marker %q", parentMarker)
		}
		parentIndex = index
	}
	if parentIndex == -1 {
		return nil, fmt.Errorf("no opening tag contains parent marker %q", parentMarker)
	}
	if tags[parentIndex].selfClosing {
		return nil, fmt.Errorf("parent element containing marker %q is self-closing", parentMarker)
	}

	stack := []string{tags[parentIndex].name}
	var children []testHTMLTagBoundary
	for _, tag := range tags[parentIndex+1:] {
		if tag.closing {
			if len(stack) == 0 || stack[len(stack)-1] != tag.name {
				return nil, fmt.Errorf("mismatched closing tag </%s> inside parent %q", tag.name, parentMarker)
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return children, nil
			}
			continue
		}
		if len(stack) == 1 {
			children = append(children, tag)
		}
		if !tag.selfClosing {
			stack = append(stack, tag.name)
		}
	}
	return nil, fmt.Errorf("parent element containing marker %q is not closed", parentMarker)
}

func findTestHTMLElementMarkups(body, openingMarker string) ([]string, error) {
	tags, err := scanTestHTMLTagBoundaries(body)
	if err != nil {
		return nil, err
	}
	var elements []string
	for openingIndex, opening := range tags {
		if opening.closing || !strings.Contains(body[opening.start:opening.end], openingMarker) {
			continue
		}
		if opening.selfClosing {
			return nil, fmt.Errorf("element containing marker %q is self-closing", openingMarker)
		}
		depth := 1
		for _, tag := range tags[openingIndex+1:] {
			if tag.name != opening.name {
				continue
			}
			if tag.closing {
				depth--
				if depth == 0 {
					elements = append(elements, body[opening.start:tag.end])
					break
				}
				continue
			}
			if !tag.selfClosing {
				depth++
			}
		}
		if depth != 0 {
			return nil, fmt.Errorf("element <%s> containing marker %q is not closed", opening.name, openingMarker)
		}
	}
	return elements, nil
}

func testHTMLTextContent(markup string) (string, error) {
	tags, err := scanTestHTMLTagBoundaries(markup)
	if err != nil {
		return "", err
	}
	var text strings.Builder
	cursor := 0
	for _, tag := range tags {
		text.WriteString(markup[cursor:tag.start])
		text.WriteByte(' ')
		cursor = tag.end
	}
	text.WriteString(markup[cursor:])
	return strings.Join(strings.Fields(html.UnescapeString(text.String())), " "), nil
}

func scanTestHTMLTagBoundaries(body string) ([]testHTMLTagBoundary, error) {
	var tags []testHTMLTagBoundary
	for cursor := 0; cursor < len(body); {
		relativeStart := strings.IndexByte(body[cursor:], '<')
		if relativeStart == -1 {
			break
		}
		start := cursor + relativeStart
		if strings.HasPrefix(body[start:], "<!--") {
			relativeEnd := strings.Index(body[start+4:], "-->")
			if relativeEnd == -1 {
				return nil, fmt.Errorf("unterminated HTML comment at byte %d", start)
			}
			cursor = start + 4 + relativeEnd + len("-->")
			continue
		}

		end, err := findTestHTMLTagEnd(body, start)
		if err != nil {
			return nil, err
		}
		content := strings.TrimSpace(body[start+1 : end-1])
		if content == "" || content[0] == '!' || content[0] == '?' {
			cursor = end
			continue
		}

		closing := content[0] == '/'
		if closing {
			content = strings.TrimSpace(content[1:])
		}
		nameEnd := 0
		for nameEnd < len(content) && isTestHTMLTagNameByte(content[nameEnd]) {
			nameEnd++
		}
		if nameEnd == 0 {
			cursor = end
			continue
		}

		tags = append(tags, testHTMLTagBoundary{
			start:       start,
			end:         end,
			name:        strings.ToLower(content[:nameEnd]),
			closing:     closing,
			selfClosing: !closing && strings.HasSuffix(strings.TrimSpace(content), "/"),
		})
		cursor = end
	}

	return tags, nil
}

func findTestHTMLTagEnd(body string, start int) (int, error) {
	var quote byte
	for index := start + 1; index < len(body); index++ {
		current := body[index]
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current == '>' {
			return index + 1, nil
		}
	}

	return 0, fmt.Errorf("unterminated HTML tag at byte %d", start)
}

func isTestHTMLTagNameByte(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		value == ':' || value == '-'
}

func assertHomeSystemsOverlook(t *testing.T, path, body string) {
	t.Helper()
	if err := validateHomeSystemsOverlookStructure(body); err != nil {
		t.Errorf("GET %s Home systems overlook structure: %v", path, err)
	}

	markers := []string{
		`data-remote-image`,
		`data-image-fallback`,
		`>CJ</span>`,
		`href="https://gravatar.com/craigdevjohnson1"`,
		`aria-label="View Craig Johnson's Gravatar profile (opens in new tab)"`,
		`width="275"`,
		`height="275"`,
		`loading="eager"`,
		`href="/experience"`,
		`href="/projects"`,
		`href="/contact"`,
		`href="/skills"`,
		`href="/soccer"`,
		`Cloud Architecture`,
		`Infrastructure as Code`,
		`Delivery Automation`,
		`Security &amp; Operations`,
	}
	for _, marker := range markers {
		if !strings.Contains(body, marker) {
			t.Errorf("GET %s Home systems overlook does not contain %q", path, marker)
		}
	}
	for _, removed := range []string{`data-region="proof"`, `home-proof-grid`, `Proof of Work`, `Follow the systems behind the résumé`} {
		if strings.Contains(body, removed) {
			t.Errorf("GET %s Home systems overlook still contains removed proof marker %q", path, removed)
		}
	}
}

func validateHomeSystemsOverlookStructure(body string) error {
	regions := []string{"photo", "intro", "topology"}
	if err := validateOrderedPageRegions(body, "systems-overlook", regions); err != nil {
		return err
	}

	if count := strings.Count(body, "data-signal-trail"); count != 1 {
		return fmt.Errorf("full-page signal trail marker count = %d, want 1", count)
	}

	topology, err := findTestHTMLElementMarkup(body, `data-region="topology"`)
	if err != nil {
		return fmt.Errorf("topology element boundaries: %w", err)
	}
	if count := strings.Count(topology, "data-signal-trail"); count != 1 {
		return fmt.Errorf("topology signal trail marker count = %d, want 1", count)
	}
	if count := strings.Count(topology, "signal-trail-topology"); count != 1 {
		return fmt.Errorf("topology trail class count = %d, want 1", count)
	}

	return nil
}

func TestValidateHomeSystemsOverlookStructureRejectsMutations(t *testing.T) {
	const valid = `<div data-layout="systems-overlook">` +
		`<figure data-region="photo"></figure>` +
		`<div data-region="intro"></div>` +
		`<section data-region="topology"><div class="signal-trail signal-trail-topology" data-signal-trail></div></section>` +
		`</div>`

	tests := []struct {
		name string
		body string
	}{
		{
			name: "duplicate named region",
			body: strings.Replace(valid, `<figure data-region="photo"></figure>`, `<figure data-region="photo"></figure><div data-region="photo"></div>`, 1),
		},
		{
			name: "unexpected fourth region",
			body: strings.Replace(valid, `<section data-region="topology">`, `<aside data-region="extra"></aside><section data-region="topology">`, 1),
		},
		{
			name: "trail before topology",
			body: strings.Replace(valid,
				`<section data-region="topology"><div class="signal-trail signal-trail-topology" data-signal-trail></div></section>`,
				`<div class="signal-trail signal-trail-topology" data-signal-trail></div><section data-region="topology"></section>`, 1),
		},
		{
			name: "trail after topology",
			body: strings.Replace(valid,
				`<section data-region="topology"><div class="signal-trail signal-trail-topology" data-signal-trail></div></section>`,
				`<section data-region="topology"></section><div class="signal-trail signal-trail-topology" data-signal-trail></div>`, 1),
		},
		{
			name: "trail inside intro",
			body: strings.Replace(valid,
				`<div data-region="intro"></div><section data-region="topology"><div class="signal-trail signal-trail-topology" data-signal-trail></div></section>`,
				`<div data-region="intro"><div class="signal-trail signal-trail-topology" data-signal-trail></div></div><section data-region="topology"></section>`, 1),
		},
		{
			name: "duplicate topology trail",
			body: strings.Replace(valid, `</section></div>`, `<div class="signal-trail signal-trail-topology" data-signal-trail></div></section></div>`, 1),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateHomeSystemsOverlookStructure(test.body); err == nil {
				t.Fatal("validateHomeSystemsOverlookStructure() error = nil, want mutation rejected")
			}
		})
	}
}

func TestValidateHomeSystemsOverlookStructureAcceptsValidMarkup(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "basic topology",
			body: `<div data-layout="systems-overlook">` +
				`<figure data-region="photo"></figure>` +
				`<div data-region="intro"></div>` +
				`<section data-region="topology"><div class="signal-trail signal-trail-topology" data-signal-trail></div></section>` +
				`</div>`,
		},
		{
			name: "nested same-name element with comment and quoted tag boundary",
			body: `<div data-layout="systems-overlook">` +
				`<figure data-region="photo"></figure>` +
				`<div data-region="intro"></div>` +
				`<section title="quoted > boundary" data-region="topology">` +
				`<!-- fake </section> and <section> tags -->` +
				`<section aria-label='nested > topology'><div></div></section>` +
				`<div class="signal-trail signal-trail-topology" data-signal-trail></div>` +
				`</section>` +
				`</div>`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateHomeSystemsOverlookStructure(test.body); err != nil {
				t.Fatalf("validateHomeSystemsOverlookStructure() error = %v, want nil", err)
			}
		})
	}
}

func assertAboutAlaskaSwitchback(t *testing.T, path, body string) {
	t.Helper()
	if err := validateAboutAlaskaSwitchbackStructure(body); err != nil {
		t.Errorf("GET %s About Alaska switchback structure: %v", path, err)
	}

	markers := []string{
		`src="/static/images/backgrounds/about-hero.jpg"`,
		`--page-kit-hero-photo-position: center 36%;`,
		`Alaska roots, wider horizons`,
		`From Alaska service desks to principal-level cloud engineering.`,
		`Years in Tech`,
		`Certifications`,
		`Technologies`,
		`Cups of Coffee`,
		`Originally from Alaska`,
		`Family of 4 + 2 dogs`,
		`Lifelong soccer enthusiast`,
		`BS in Cloud Computing`,
		`Cloud &amp; automation specialist`,
		`Continuous learner`,
		`Started in Tech`,
		`Healthcare IT`,
		`Microsoft Certified`,
		`Cloud Transition`,
		`Cloud Engineer Principal`,
		`Leaving Alaska expanded both the environments`,
		`My tech career began at the local university`,
		`Soccer has been a lifelong love of mine`,
		`Playing and coaching`,
		`Alaska roots mean a love for hiking`,
		`Strategy and building games exercise`,
		`A lively home office comes with four paws`,
		`Iterate, measure, and leave the system easier`,
		`Build with the people who operate the platform`,
		`Break complex constraints into practical`,
		`Make context durable so the next person`,
		`href="/experience"`,
		`View My Experience`,
		`href="/contact"`,
		`Get in Touch`,
	}
	for _, marker := range markers {
		if !strings.Contains(body, marker) {
			t.Errorf("GET %s About Alaska switchback does not contain %q", path, marker)
		}
	}

	hero, err := findTestHTMLElementMarkup(body, `class="page-kit-shell page-kit-hero`)
	if err != nil {
		t.Errorf("GET %s About hero boundaries: %v", path, err)
	} else {
		if strings.Contains(hero, "min-h-full") {
			t.Errorf("GET %s About hero retains min-h-full utility that clips stacked hero content", path)
		}
		if !strings.Contains(hero, "about-hero-content") {
			t.Errorf("GET %s About hero lacks its route-owned content layout class", path)
		}
	}

	facts, err := findTestHTMLElementMarkup(body, `data-region="facts"`)
	if err != nil {
		t.Errorf("GET %s About facts boundaries: %v", path, err)
	} else {
		if !strings.Contains(facts, "<h2") || !strings.Contains(facts, "<ul") {
			t.Errorf("GET %s About facts are not headed list content", path)
		}
		if count := strings.Count(facts, "<li"); count != 6 {
			t.Errorf("GET %s About fact count = %d, want 6", path, count)
		}
	}

	timeline, err := findTestHTMLElementMarkup(body, `data-region="timeline"`)
	if err != nil {
		t.Errorf("GET %s About timeline boundaries: %v", path, err)
	} else {
		if !strings.Contains(timeline, "<ol") {
			t.Errorf("GET %s About timeline is not an ordered list", path)
		}
		if count := strings.Count(timeline, `<li class="about-timeline-entry`); count != 6 {
			t.Errorf("GET %s About timeline step count = %d, want 6", path, count)
		}
		if count := strings.Count(timeline, `aria-current="step"`); count != 1 {
			t.Errorf("GET %s About current timeline step count = %d, want 1", path, count)
		}
	}
}

func validateAboutAlaskaSwitchbackStructure(body string) error {
	regions := []string{"story", "facts", "timeline", "hobbies", "values"}
	if err := validateOrderedPageRegions(body, "alaska-switchback", regions); err != nil {
		return err
	}
	if count := strings.Count(body, "<h1"); count != 1 {
		return fmt.Errorf("h1 count = %d, want 1", count)
	}
	if count := strings.Count(body, "data-signal-trail"); count != 1 {
		return fmt.Errorf("full-page signal trail marker count = %d, want 1", count)
	}

	switchback, err := findTestHTMLElementMarkup(body, `data-about-switchback`)
	if err != nil {
		return fmt.Errorf("switchback element boundaries: %w", err)
	}
	story, err := findTestHTMLElementBounds(switchback, `data-region="story"`)
	if err != nil {
		return fmt.Errorf("story element boundaries: %w", err)
	}
	facts, err := findTestHTMLElementBounds(switchback, `data-region="facts"`)
	if err != nil {
		return fmt.Errorf("facts element boundaries: %w", err)
	}
	if story.end > facts.start {
		return fmt.Errorf("facts are nested in or ordered before the closed story element")
	}
	if gap := switchback[story.end:facts.start]; !testHTMLGapIsIgnorable(gap) {
		return fmt.Errorf("story and facts are not adjacent siblings; intervening markup = %q", strings.TrimSpace(gap))
	}

	timeline, err := findTestHTMLElementBounds(switchback, `data-region="timeline"`)
	if err != nil {
		return fmt.Errorf("timeline element boundaries: %w", err)
	}
	if facts.end > timeline.start {
		return fmt.Errorf("timeline is nested in or ordered before the closed facts element")
	}
	if gap := switchback[facts.end:timeline.start]; !testHTMLGapIsIgnorable(gap) {
		return fmt.Errorf("facts and timeline are not adjacent siblings; intervening markup = %q", strings.TrimSpace(gap))
	}

	timelineMarkup := switchback[timeline.start:timeline.end]
	timelineComposition, err := findTestHTMLElementMarkup(timelineMarkup, `class="about-timeline"`)
	if err != nil {
		return fmt.Errorf("timeline composition boundaries: %w", err)
	}
	trail, err := findTestHTMLElementBounds(timelineComposition, `signal-trail-switchback`)
	if err != nil {
		return fmt.Errorf("timeline background trail boundaries: %w", err)
	}
	timelineList, err := findTestHTMLElementBounds(timelineComposition, `<ol`)
	if err != nil {
		return fmt.Errorf("timeline list boundaries: %w", err)
	}
	if trail.end > timelineList.start {
		return fmt.Errorf("timeline background trail must precede the timeline cards")
	}
	if strings.Contains(switchback, `data-story-path="switchback-transition"`) {
		return fmt.Errorf("switchback retains a labeled trail separator")
	}
	if count := strings.Count(switchback, "signal-trail-switchback"); count != 1 {
		return fmt.Errorf("switchback background trail class count = %d, want 1", count)
	}

	return nil
}

func testHTMLGapIsIgnorable(gap string) bool {
	for {
		gap = strings.TrimSpace(gap)
		if gap == "" {
			return true
		}
		if !strings.HasPrefix(gap, "<!--") {
			return false
		}
		end := strings.Index(gap[len("<!--"):], "-->")
		if end == -1 {
			return false
		}
		gap = gap[len("<!--")+end+len("-->"):]
	}
}

func TestValidateAboutAlaskaSwitchbackStructureRejectsMutations(t *testing.T) {
	const trail = `<div class="signal-trail signal-trail-switchback" data-signal-trail></div>`
	const valid = `<div data-layout="alaska-switchback"><h1>About Me</h1>` +
		`<div data-about-switchback>` +
		`<section data-region="story"></section>` +
		`<aside data-region="facts"></aside>` +
		`<section data-region="timeline"><div class="about-timeline">` + trail + `<ol></ol></div></section>` +
		`</div>` +
		`<section data-region="hobbies"></section>` +
		`<section data-region="values"></section>` +
		`</div>`

	tests := []struct {
		name string
		body string
	}{
		{name: "missing region", body: strings.Replace(valid, `<section data-region="values"></section>`, ``, 1)},
		{name: "wrong region", body: strings.Replace(valid, `data-region="values"`, `data-region="principles"`, 1)},
		{name: "duplicate named region", body: strings.Replace(valid, `<section data-region="values">`, `<section data-region="hobbies"></section><section data-region="values">`, 1)},
		{name: "unexpected region", body: strings.Replace(valid, `<section data-region="values">`, `<aside data-region="extra"></aside><section data-region="values">`, 1)},
		{name: "facts before story", body: strings.Replace(valid, `<section data-region="story"></section><aside data-region="facts"></aside>`, `<aside data-region="facts"></aside><section data-region="story"></section>`, 1)},
		{name: "facts after timeline", body: strings.Replace(valid, `<aside data-region="facts"></aside><section data-region="timeline"><div class="about-timeline">`+trail+`<ol></ol></div></section>`, `<section data-region="timeline"><div class="about-timeline">`+trail+`<ol></ol></div></section><aside data-region="facts"></aside>`, 1)},
		{name: "missing trail", body: strings.Replace(valid, trail, ``, 1)},
		{name: "duplicate trail", body: strings.Replace(valid, trail, trail+trail, 1)},
		{name: "wrong trail", body: strings.Replace(valid, `signal-trail-switchback`, `signal-trail-topology`, 1)},
		{name: "trail outside timeline composition", body: strings.Replace(valid, `<div class="about-timeline">`+trail, trail+`<div class="about-timeline">`, 1)},
		{name: "trail after timeline cards", body: strings.Replace(valid, trail+`<ol></ol>`, `<ol></ol>`+trail, 1)},
		{name: "separator restored", body: strings.Replace(valid, trail, `<div data-story-path="switchback-transition">`+trail+`</div>`, 1)},
		{name: "duplicate h1", body: strings.Replace(valid, `<h1>About Me</h1>`, `<h1>About Me</h1><h1>Again</h1>`, 1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateAboutAlaskaSwitchbackStructure(test.body); err == nil {
				t.Fatal("validateAboutAlaskaSwitchbackStructure() error = nil, want mutation rejected")
			}
		})
	}
}

func TestValidateAboutAlaskaSwitchbackStructureAcceptsValidNestedMarkup(t *testing.T) {
	const body = `<main title="quoted > boundary" data-layout="alaska-switchback"><h1>About Me</h1>` +
		`<div data-about-switchback>` +
		`<section data-region="story"><section aria-label='nested > story'></section></section>` +
		`<!-- story and facts remain adjacent -->` +
		`<aside data-region="facts"></aside>` +
		`<!-- facts and timeline remain adjacent -->` +
		`<section data-region="timeline"><div class="about-timeline">` +
		`<div class="signal-trail signal-trail-switchback" data-signal-trail></div><ol></ol></div></section>` +
		`</div>` +
		`<section data-region="hobbies"></section>` +
		`<section data-region="values"></section>` +
		`</main>`

	if err := validateAboutAlaskaSwitchbackStructure(body); err != nil {
		t.Fatalf("validateAboutAlaskaSwitchbackStructure() error = %v, want nil", err)
	}
}

func assertExperienceCareerEras(t *testing.T, path, body string) {
	t.Helper()
	if err := validateExperienceCareerEraStructure(body); err != nil {
		t.Errorf("GET %s Experience career-era structure: %v", path, err)
	}
	if err := validateExperienceRoleContent(body); err != nil {
		t.Errorf("GET %s Experience role content: %v", path, err)
	}

	markers := []string{
		`src="/static/images/backgrounds/experience-hero.jpg"`,
		`Systems, teams, and the work between`,
		`Experience in Three Eras`,
		`A chaptered view of my career progression`,
		`Years active`,
		`Roles`,
		`Technologies`,
		`href="#foundation"`,
		`href="#systems-growth"`,
		`href="#cloud-leadership"`,
		`PowerShell`,
		`AD DS`,
		`Windows`,
		`AWS`,
		`Ansible`,
		`Azure`,
		`Bash`,
		`Go`,
		`Cloud Engineer Principal`,
		`COMPANY REDACTED - A`,
		`2022 – Present`,
		`Lead infrastructure automation initiatives using IaC principles.`,
		`System Administrator`,
		`COMPANY REDACTED - B`,
		`2021 – 2022`,
		`Managed enterprise SCADA systems and infrastructure automation.`,
		`IT Systems Engineer Sr`,
		`2020 – 2021`,
		`Architected and implemented cloud infrastructure solutions in healthcare environments.`,
		`IT Systems Engineer`,
		`2018 – 2020`,
		`Managed enterprise Active Directory and Exchange infrastructure.`,
		`IT Desktop Engineer`,
		`2017 – 2018`,
		`Implemented automated solutions for endpoint management and configuration.`,
		`IT Service Desk Associate`,
		`2016 – 2017`,
		`Utilized ITSM platforms for incident and change management.`,
		`Service Desk Student Analyst`,
		`COMPANY REDACTED - D`,
		`2012 – 2016`,
		`Managed incident tracking through enterprise ITSM systems.`,
		`href="/projects"`,
		`View Projects`,
		`href="/skills"`,
		`Review Skills`,
	}
	for _, marker := range markers {
		if !strings.Contains(body, marker) {
			t.Errorf("GET %s Experience career eras do not contain %q", path, marker)
		}
	}

	if count := strings.Count(body, `data-experience-summary-stat`); count != 3 {
		t.Errorf("GET %s Experience summary stat count = %d, want 3", path, count)
	}
	if count := strings.Count(body, `data-recurring-technology`); count != 8 {
		t.Errorf("GET %s recurring technology count = %d, want 8", path, count)
	}
	if count := strings.Count(body, `data-role-technology`); count != 23 {
		t.Errorf("GET %s rendered role technology count = %d, want 23", path, count)
	}
	if count := strings.Count(body, `stat-grid-summary`); count != 1 {
		t.Errorf("GET %s StatGridSummary marker count = %d, want 1", path, count)
	}
	if count := strings.Count(body, `class="experience-era-title-row"`); count != 3 {
		t.Errorf("GET %s era heading/date-status row count = %d, want 3", path, count)
	}
	if count := strings.Count(body, `class="experience-role-title-row"`); count != 7 {
		t.Errorf("GET %s role heading/date row count = %d, want 7", path, count)
	}

	assertExperienceRoleCardsUseNaturalHeight(t, path, body)
	assertExperienceLayoutIsRouteOwned(t, path, body)
}

func assertExperienceLayoutIsRouteOwned(t *testing.T, path, body string) {
	t.Helper()
	tags, err := scanTestHTMLTagBoundaries(body)
	if err != nil {
		t.Errorf("GET %s scan Experience layout owners: %v", path, err)
		return
	}
	mainCount := 0
	for _, tag := range tags {
		if !tag.closing && tag.name == "main" {
			mainCount++
		}
	}
	if mainCount != 1 {
		t.Errorf("GET %s main landmark count = %d, want exactly one", path, mainCount)
	}
	markers := []string{
		`data-layout="career-eras"`,
		`data-experience-summary-grid`,
		`data-experience-spotlight`,
		`data-career-sequence`,
		`data-career-era="`,
		`data-era-layout`,
		`data-era-heading`,
		`data-role-list`,
	}
	for _, tag := range tags {
		if tag.closing {
			continue
		}
		opening := body[tag.start:tag.end]
		owned := false
		for _, marker := range markers {
			if strings.Contains(opening, marker) {
				owned = true
				break
			}
		}
		if !owned {
			continue
		}
		for _, className := range testHTMLClassTokens(opening) {
			if strings.Contains(className, ":") || testExperienceLayoutUtility(className) {
				t.Errorf("GET %s Experience route-owned element carries layout utility %q", path, className)
			}
		}
	}
}

func testExperienceLayoutUtility(className string) bool {
	for _, exact := range []string{"grid", "flex", "relative", "absolute", "overflow-hidden", "h-full", "min-h-full"} {
		if className == exact {
			return true
		}
	}
	for _, prefix := range []string{"grid-", "gap-", "space-", "p-", "px-", "py-", "m-", "mx-", "my-", "order-", "items-", "self-"} {
		if strings.HasPrefix(className, prefix) {
			return true
		}
	}
	return false
}

func assertExperienceRoleCardsUseNaturalHeight(t *testing.T, path, body string) {
	t.Helper()
	tags, err := scanTestHTMLTagBoundaries(body)
	if err != nil {
		t.Errorf("GET %s scan Experience role cards: %v", path, err)
		return
	}
	for _, tag := range tags {
		if tag.closing {
			continue
		}
		opening := body[tag.start:tag.end]
		if !strings.Contains(opening, `data-experience-role="`) {
			continue
		}
		for _, className := range testHTMLClassTokens(opening) {
			switch className {
			case "h-full", "min-h-full", "self-stretch", "items-stretch", "place-items-stretch":
				t.Errorf("GET %s Experience role card carries equal-height class %q", path, className)
			}
		}
	}
}

func testHTMLClassTokens(openingTag string) []string {
	const marker = `class="`
	start := strings.Index(openingTag, marker)
	if start == -1 {
		return nil
	}
	start += len(marker)
	end := strings.IndexByte(openingTag[start:], '"')
	if end == -1 {
		return nil
	}
	return strings.Fields(openingTag[start : start+end])
}

func validateExperienceCareerEraStructure(body string) error {
	if marker := `data-layout="career-eras"`; strings.Count(body, marker) != 1 {
		return fmt.Errorf("layout marker %q count = %d, want 1", marker, strings.Count(body, marker))
	}
	if count := strings.Count(body, "<h1"); count != 1 {
		return fmt.Errorf("h1 count = %d, want 1", count)
	}
	if count := strings.Count(body, `data-signal-trail`); count != 2 {
		return fmt.Errorf("full-page signal trail count = %d, want 2", count)
	}
	if count := strings.Count(body, `data-career-era="`); count != 3 {
		return fmt.Errorf("career-era marker count = %d, want 3", count)
	}
	if count := strings.Count(body, `data-experience-role="`); count != 7 {
		return fmt.Errorf("experience role marker count = %d, want 7", count)
	}

	sequence, err := findTestHTMLElementMarkup(body, `data-career-sequence`)
	if err != nil {
		return fmt.Errorf("career sequence boundaries: %w", err)
	}
	if !strings.Contains(sequence, "<ol") {
		return fmt.Errorf("career sequence is not an ordered list")
	}
	if count := strings.Count(sequence, `data-signal-trail`); count != 1 {
		return fmt.Errorf("career sequence signal trail count = %d, want 1", count)
	}
	if count := strings.Count(sequence, `signal-trail-timeline`); count != 1 {
		return fmt.Errorf("career sequence typed timeline trail count = %d, want 1", count)
	}
	orientation, err := findTestHTMLElementMarkup(body, `class="experience-orientation"`)
	if err != nil {
		return fmt.Errorf("experience orientation boundaries: %w", err)
	}
	if count := strings.Count(orientation, `data-signal-trail`); count != 1 {
		return fmt.Errorf("experience orientation signal trail count = %d, want 1", count)
	}
	if count := strings.Count(orientation, `signal-trail-topology`); count != 1 {
		return fmt.Errorf("experience orientation typed topology trail count = %d, want 1", count)
	}
	eraListOpening, err := findTestHTMLElementOpeningTag(sequence, `class="experience-era-list"`)
	if err != nil {
		return fmt.Errorf("career era list opening: %w", err)
	}
	if eraListOpening.name != "ol" {
		return fmt.Errorf("career era list element = <%s>, want <ol>", eraListOpening.name)
	}
	eraList, err := findTestHTMLElementMarkup(sequence, `class="experience-era-list"`)
	if err != nil {
		return fmt.Errorf("career era list boundaries: %w", err)
	}
	eraItems, err := findTestHTMLDirectChildOpenings(eraList, `class="experience-era-list"`)
	if err != nil {
		return fmt.Errorf("career era list children: %w", err)
	}
	if len(eraItems) != 3 {
		return fmt.Errorf("career era list direct-child count = %d, want 3", len(eraItems))
	}

	expectedEras := []struct {
		id         string
		title      string
		status     string
		roleIDs    []string
		roleTitles []string
		roleCount  int
	}{
		{id: "foundation", title: "Foundation", status: "completed", roleIDs: []string{"7", "6"}, roleTitles: []string{"Service Desk Student Analyst", "IT Service Desk Associate"}, roleCount: 2},
		{id: "systems-growth", title: "Systems Growth", status: "completed", roleIDs: []string{"5", "4", "3"}, roleTitles: []string{"IT Desktop Engineer", "IT Systems Engineer", "IT Systems Engineer Sr"}, roleCount: 3},
		{id: "cloud-leadership", title: "Cloud Leadership", status: "current", roleIDs: []string{"2", "1"}, roleTitles: []string{"System Administrator", "Cloud Engineer Principal"}, roleCount: 2},
	}
	for eraIndex, expected := range expectedEras {
		eraMarker := `data-career-era="` + expected.id + `"`
		eraOpening := eraItems[eraIndex]
		if eraOpening.name != "li" {
			return fmt.Errorf("career era %q direct child = <%s>, want <li>", expected.id, eraOpening.name)
		}
		if opening := eraList[eraOpening.start:eraOpening.end]; strings.Count(opening, eraMarker) != 1 {
			return fmt.Errorf("career era list child %d does not carry marker %q", eraIndex, expected.id)
		}

		era, eraErr := findTestHTMLElementMarkup(sequence, eraMarker)
		if eraErr != nil {
			return fmt.Errorf("career era %q boundaries: %w", expected.id, eraErr)
		}
		eraHeading, headingErr := findTestHTMLElementOpeningTag(era, `class="experience-era-title"`)
		if headingErr != nil {
			return fmt.Errorf("career era %q heading: %w", expected.id, headingErr)
		}
		if eraHeading.name != "h3" {
			return fmt.Errorf("career era %q heading element = <%s>, want <h3>", expected.id, eraHeading.name)
		}
		eraHeadingMarkup, headingMarkupErr := findTestHTMLElementMarkup(era, `class="experience-era-title"`)
		if headingMarkupErr != nil {
			return fmt.Errorf("career era %q heading boundaries: %w", expected.id, headingMarkupErr)
		}
		eraHeadingText, textErr := testHTMLTextContent(eraHeadingMarkup)
		if textErr != nil {
			return fmt.Errorf("career era %q heading text: %w", expected.id, textErr)
		}
		if eraHeadingText != expected.title {
			return fmt.Errorf("career era %q heading text = %q, want %q", expected.id, eraHeadingText, expected.title)
		}
		if count := strings.Count(era, `data-experience-role="`); count != expected.roleCount {
			return fmt.Errorf("career era %q role count = %d, want %d", expected.id, count, expected.roleCount)
		}
		if count := strings.Count(era, `data-era-status="`+expected.status+`"`); count != 1 {
			return fmt.Errorf("career era %q status %q count = %d, want 1", expected.id, expected.status, count)
		}
		roleListOpening, roleListErr := findTestHTMLElementOpeningTag(era, `class="experience-role-list"`)
		if roleListErr != nil {
			return fmt.Errorf("career era %q role list: %w", expected.id, roleListErr)
		}
		if roleListOpening.name != "ol" {
			return fmt.Errorf("career era %q role list element = <%s>, want <ol>", expected.id, roleListOpening.name)
		}
		roleList, roleListMarkupErr := findTestHTMLElementMarkup(era, `class="experience-role-list"`)
		if roleListMarkupErr != nil {
			return fmt.Errorf("career era %q role list boundaries: %w", expected.id, roleListMarkupErr)
		}
		previousRole := -1
		for roleIndex, roleID := range expected.roleIDs {
			roleMarker := `data-experience-role="` + roleID + `"`
			if count := strings.Count(roleList, roleMarker); count != 1 {
				return fmt.Errorf("career era %q role %s count = %d, want 1", expected.id, roleID, count)
			}
			markerIndex := strings.Index(roleList, roleMarker)
			if markerIndex <= previousRole {
				return fmt.Errorf("experience role %s is out of chronological order", roleID)
			}
			previousRole = markerIndex
			role, roleErr := findTestHTMLElementMarkup(roleList, roleMarker)
			if roleErr != nil {
				return fmt.Errorf("experience role %s boundaries: %w", roleID, roleErr)
			}
			roleOpening, roleOpeningErr := findTestHTMLElementOpeningTag(roleList, roleMarker)
			if roleOpeningErr != nil {
				return fmt.Errorf("experience role %s opening: %w", roleID, roleOpeningErr)
			}
			if roleOpening.name != "article" {
				return fmt.Errorf("experience role %s element = <%s>, want <article>", roleID, roleOpening.name)
			}
			roleHeading, roleHeadingErr := findTestHTMLElementOpeningTag(role, `class="experience-role-title"`)
			if roleHeadingErr != nil {
				return fmt.Errorf("experience role %s heading: %w", roleID, roleHeadingErr)
			}
			if roleHeading.name != "h4" {
				return fmt.Errorf("experience role %s heading element = <%s>, want <h4>", roleID, roleHeading.name)
			}
			roleHeadingMarkup, roleHeadingMarkupErr := findTestHTMLElementMarkup(role, `class="experience-role-title"`)
			if roleHeadingMarkupErr != nil {
				return fmt.Errorf("experience role %s heading boundaries: %w", roleID, roleHeadingMarkupErr)
			}
			roleHeadingText, roleHeadingTextErr := testHTMLTextContent(roleHeadingMarkup)
			if roleHeadingTextErr != nil {
				return fmt.Errorf("experience role %s heading text: %w", roleID, roleHeadingTextErr)
			}
			if roleHeadingText != expected.roleTitles[roleIndex] {
				return fmt.Errorf("experience role %s heading text = %q, want %q", roleID, roleHeadingText, expected.roleTitles[roleIndex])
			}
		}
	}
	return nil
}

func TestValidateExperienceCareerEraStructureRejectsMutations(t *testing.T) {
	const trail = `<div class="signal-trail signal-trail-timeline" data-signal-trail></div>`
	const orientationTrail = `<div class="signal-trail signal-trail-topology experience-orientation-trail" data-signal-trail></div>`
	stats := `<div class="experience-overview-stats stat-grid-summary">` + strings.Repeat(`<div data-experience-summary-stat></div>`, 3) + `</div>`
	const foundation = `<li data-career-era="foundation"><section data-era-status="completed"><h3 class="experience-era-title">Foundation</h3><ol class="experience-role-list"><li><article data-experience-role="7"><h4 class="experience-role-title">Service Desk Student Analyst</h4></article></li><li><article data-experience-role="6"><h4 class="experience-role-title">IT Service Desk Associate</h4></article></li></ol></section></li>`
	const foundationDiv = `<div data-career-era="foundation"><section data-era-status="completed"><h3 class="experience-era-title">Foundation</h3><ol class="experience-role-list"><li><article data-experience-role="7"><h4 class="experience-role-title">Service Desk Student Analyst</h4></article></li><li><article data-experience-role="6"><h4 class="experience-role-title">IT Service Desk Associate</h4></article></li></ol></section></div>`
	const systems = `<li data-career-era="systems-growth"><section data-era-status="completed"><h3 class="experience-era-title">Systems Growth</h3><ol class="experience-role-list"><li><article data-experience-role="5"><h4 class="experience-role-title">IT Desktop Engineer</h4></article></li><li><article data-experience-role="4"><h4 class="experience-role-title">IT Systems Engineer</h4></article></li><li><article data-experience-role="3"><h4 class="experience-role-title">IT Systems Engineer Sr</h4></article></li></ol></section></li>`
	const cloud = `<li data-career-era="cloud-leadership"><section data-era-status="current"><h3 class="experience-era-title">Cloud Leadership</h3><ol class="experience-role-list"><li><article data-experience-role="2"><h4 class="experience-role-title">System Administrator</h4></article></li><li><article data-experience-role="1"><h4 class="experience-role-title">Cloud Engineer Principal</h4></article></li></ol></section></li>`
	valid := `<main data-layout="career-eras"><h1>Experience</h1><div class="experience-orientation">` + orientationTrail + `</div>` + stats + `<div data-career-sequence>` + trail + `<ol class="experience-era-list">` + foundation + systems + cloud + `</ol></div></main>`
	outerListAsDiv := strings.Replace(valid, `<ol class="experience-era-list">`, `<div class="experience-era-list">`, 1)
	outerListAsDiv = strings.Replace(outerListAsDiv, `</ol></div></main>`, `</div></div></main>`, 1)

	tests := []struct {
		name string
		body string
	}{
		{name: "missing era", body: strings.Replace(valid, systems, ``, 1)},
		{name: "wrong era", body: strings.Replace(valid, `data-career-era="systems-growth"`, `data-career-era="operations"`, 1)},
		{name: "duplicate era", body: strings.Replace(valid, cloud, cloud+cloud, 1)},
		{name: "eras reordered", body: strings.Replace(valid, foundation+systems, systems+foundation, 1)},
		{name: "empty era", body: strings.Replace(valid, `<ol class="experience-role-list"><li><article data-experience-role="7"><h4 class="experience-role-title">Service Desk Student Analyst</h4></article></li><li><article data-experience-role="6"><h4 class="experience-role-title">IT Service Desk Associate</h4></article></li></ol>`, `<ol class="experience-role-list"></ol>`, 1)},
		{name: "missing role", body: strings.Replace(valid, `<li><article data-experience-role="4"><h4 class="experience-role-title">IT Systems Engineer</h4></article></li>`, ``, 1)},
		{name: "duplicate role", body: strings.Replace(valid, `<li><article data-experience-role="4"><h4 class="experience-role-title">IT Systems Engineer</h4></article></li>`, strings.Repeat(`<li><article data-experience-role="4"><h4 class="experience-role-title">IT Systems Engineer</h4></article></li>`, 2), 1)},
		{name: "roles reordered", body: strings.Replace(valid, `<li><article data-experience-role="5"><h4 class="experience-role-title">IT Desktop Engineer</h4></article></li><li><article data-experience-role="4"><h4 class="experience-role-title">IT Systems Engineer</h4></article></li>`, `<li><article data-experience-role="4"><h4 class="experience-role-title">IT Systems Engineer</h4></article></li><li><article data-experience-role="5"><h4 class="experience-role-title">IT Desktop Engineer</h4></article></li>`, 1)},
		{name: "wrong stage status", body: strings.Replace(valid, `data-era-status="current"`, `data-era-status="completed"`, 1)},
		{name: "outer era list demoted to div despite nested role lists", body: outerListAsDiv},
		{name: "era marker is not a list item", body: strings.Replace(valid, foundation, foundationDiv, 1)},
		{name: "era heading demoted from h3", body: strings.Replace(valid, `<h3 class="experience-era-title">Foundation</h3>`, `<p class="experience-era-title">Foundation</p>`, 1)},
		{name: "role heading demoted from h4", body: strings.Replace(valid, `<h4 class="experience-role-title">Cloud Engineer Principal</h4>`, `<p class="experience-role-title">Cloud Engineer Principal</p>`, 1)},
		{name: "missing trail", body: strings.Replace(valid, trail, ``, 1)},
		{name: "duplicate trail", body: strings.Replace(valid, trail, trail+trail, 1)},
		{name: "wrong trail", body: strings.Replace(valid, `signal-trail-timeline`, `signal-trail-switchback`, 1)},
		{name: "missing orientation trail", body: strings.Replace(valid, orientationTrail, ``, 1)},
		{name: "wrong orientation trail", body: strings.Replace(valid, `signal-trail-topology`, `signal-trail-dossier`, 1)},
		{name: "trail sibling before closed sequence", body: strings.Replace(valid, `<div data-career-sequence>`+trail, trail+`<div data-career-sequence>`, 1)},
		{name: "trail sibling outside closed sequence", body: strings.Replace(valid, `</ol></div></main>`, `</ol></div>`+trail+`</main>`, 1)},
		{name: "duplicate h1", body: strings.Replace(valid, `<h1>Experience</h1>`, `<h1>Experience</h1><h1>Again</h1>`, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateExperienceCareerEraStructure(test.body); err == nil {
				t.Fatal("validateExperienceCareerEraStructure() error = nil, want mutation rejected")
			}
		})
	}

	if err := validateExperienceCareerEraStructure(valid); err != nil {
		t.Fatalf("validateExperienceCareerEraStructure(valid) error = %v", err)
	}
}

func TestValidateExperienceCareerEraStructureAcceptsNestedMarkup(t *testing.T) {
	const body = `<main data-layout="career-eras"><h1>Experience</h1>` +
		`<div class="experience-orientation"><div class="signal-trail signal-trail-topology experience-orientation-trail" data-signal-trail></div></div>` +
		`<div data-career-sequence title="quoted > boundary">` +
		`<div class="signal-trail signal-trail-timeline" data-signal-trail></div><ol class="experience-era-list">` +
		`<li data-career-era="foundation"><section data-era-status="completed"><h3 class="experience-era-title">Foundation</h3><div><!-- fake </li> --></div><ol class="experience-role-list"><li><article data-experience-role="7"><h4 class="experience-role-title">Service Desk Student Analyst</h4></article></li><li><article data-experience-role="6"><h4 class="experience-role-title">IT Service Desk Associate</h4></article></li></ol></section></li>` +
		`<li data-career-era="systems-growth"><section data-era-status="completed"><h3 class="experience-era-title">Systems Growth</h3><ol class="experience-role-list"><li><article data-experience-role="5"><h4 class="experience-role-title">IT Desktop Engineer</h4></article></li><li><article data-experience-role="4"><h4 class="experience-role-title">IT Systems Engineer</h4></article></li><li><article data-experience-role="3"><h4 class="experience-role-title">IT Systems Engineer Sr</h4></article></li></ol></section></li>` +
		`<li data-career-era="cloud-leadership"><section data-era-status="current"><h3 class="experience-era-title">Cloud Leadership</h3><ol class="experience-role-list"><li><article data-experience-role="2"><h4 class="experience-role-title">System Administrator</h4></article></li><li><article data-experience-role="1"><h4 class="experience-role-title">Cloud Engineer Principal</h4></article></li></ol></section></li>` +
		`</ol></div></main>`
	if err := validateExperienceCareerEraStructure(body); err != nil {
		t.Fatalf("validateExperienceCareerEraStructure() error = %v, want nil", err)
	}
}

func TestValidateExperienceRoleContentRejectsMutations(t *testing.T) {
	body := renderExperienceRouteBody(t)
	tests := []struct {
		name string
		body string
	}{
		{
			name: "responsibility substitution inside bounded role",
			body: strings.Replace(body,
				`Lead infrastructure automation initiatives using IaC principles. Implement CI/CD pipelines for application deployment and configuration management. Architect and maintain cloud-native solutions while optimizing application performance and security. Develop self-service capabilities through automation, reducing deployment time by implementing GitOps methodologies.`,
				`Lead platform initiatives. Implement CI/CD pipelines for application deployment and configuration management. Architect and maintain cloud-native solutions while optimizing application performance and security. Develop self-service capabilities through automation, reducing deployment time by implementing GitOps methodologies.`, 1),
		},
		{
			name: "technology substitution inside bounded role",
			body: strings.Replace(body, `>Terraform</span>`, `>Pulumi</span>`, 1),
		},
		{
			name: "technology removal inside bounded role",
			body: strings.Replace(body, `<li><span class="page-kit-chip" data-role-technology>Terraform</span></li>`, ``, 1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateExperienceRoleContent(test.body); err == nil {
				t.Fatal("validateExperienceRoleContent() error = nil, want mutation rejected")
			}
		})
	}
	if err := validateExperienceRoleContent(body); err != nil {
		t.Fatalf("validateExperienceRoleContent(valid route) error = %v", err)
	}
}

func renderExperienceRouteBody(t *testing.T) string {
	t.Helper()
	application := newTestApp(t)
	mux, _ := buildMux(application, application.Logger, true)
	request := httptest.NewRequest(http.MethodGet, "/experience", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /experience status = %d, want %d", response.Code, http.StatusOK)
	}
	return response.Body.String()
}

func validateExperienceRoleContent(body string) error {
	expected := []struct {
		id               string
		title            string
		company          string
		duration         string
		responsibilities string
		technologies     []string
	}{
		{id: "7", title: "Service Desk Student Analyst", company: "COMPANY REDACTED - D", duration: "2012 – 2016", responsibilities: "Managed incident tracking through enterprise ITSM systems. Maintained technical documentation and knowledge base articles. Achieved consistent high-quality metrics in service delivery.", technologies: []string{"Windows", "MacOS", "GoogleApps"}},
		{id: "6", title: "IT Service Desk Associate", company: "COMPANY REDACTED - C", duration: "2016 – 2017", responsibilities: "Utilized ITSM platforms for incident and change management. Maintained documentation for standard operating procedures. Provided technical support for enterprise applications and systems.", technologies: []string{"ServiceNow", "O365", "Windows"}},
		{id: "5", title: "IT Desktop Engineer", company: "COMPANY REDACTED - C", duration: "2017 – 2018", responsibilities: "Implemented automated solutions for endpoint management and configuration. Managed incident response for business-critical systems using ITIL methodologies. Established standardized deployment procedures for enterprise endpoints.", technologies: []string{"PowerShell", "SCCM", "Intune"}},
		{id: "4", title: "IT Systems Engineer", company: "COMPANY REDACTED - C", duration: "2018 – 2020", responsibilities: "Managed enterprise Active Directory and Exchange infrastructure. Implemented automation solutions for service deployment and configuration management. Orchestrated application lifecycle management and infrastructure upgrades.", technologies: []string{"PowerShell", "AD DS", "O365/Exchange"}},
		{id: "3", title: "IT Systems Engineer Sr", company: "COMPANY REDACTED - C", duration: "2020 – 2021", responsibilities: "Architected and implemented cloud infrastructure solutions in healthcare environments. Led technical projects involving cross-functional teams and vendor integration. Developed automation frameworks for critical systems and established best practices for infrastructure management.", technologies: []string{"Azure", "AD DS", "PowerShell"}},
		{id: "2", title: "System Administrator", company: "COMPANY REDACTED - B", duration: "2021 – 2022", responsibilities: "Managed enterprise SCADA systems and infrastructure automation. Implemented monitoring solutions and maintained high-availability environments. Established IT/OT integration practices while ensuring regulatory compliance. Orchestrated application deployments and infrastructure upgrades in critical environments.", technologies: []string{"IoT", "SCADA", "RHEL", "Bash"}},
		{id: "1", title: "Cloud Engineer Principal", company: "COMPANY REDACTED - A", duration: "2022 – Present", responsibilities: "Lead infrastructure automation initiatives using IaC principles. Implement CI/CD pipelines for application deployment and configuration management. Architect and maintain cloud-native solutions while optimizing application performance and security. Develop self-service capabilities through automation, reducing deployment time by implementing GitOps methodologies.", technologies: []string{"AWS", "Go", "Terraform", "Ansible"}},
	}
	for _, role := range expected {
		marker := `data-experience-role="` + role.id + `"`
		markup, err := findTestHTMLElementMarkup(body, marker)
		if err != nil {
			return fmt.Errorf("experience role %s boundaries: %w", role.id, err)
		}
		textChecks := []struct {
			label  string
			marker string
			want   string
		}{
			{label: "title", marker: `class="experience-role-title"`, want: role.title},
			{label: "company", marker: `class="experience-role-company"`, want: role.company},
			{label: "duration", marker: `class="experience-role-duration"`, want: role.duration},
			{label: "responsibilities", marker: `class="experience-role-responsibilities"`, want: role.responsibilities},
		}
		for _, check := range textChecks {
			part, partErr := findTestHTMLElementMarkup(markup, check.marker)
			if partErr != nil {
				return fmt.Errorf("experience role %s %s: %w", role.id, check.label, partErr)
			}
			got, textErr := testHTMLTextContent(part)
			if textErr != nil {
				return fmt.Errorf("experience role %s %s text: %w", role.id, check.label, textErr)
			}
			if got != check.want {
				return fmt.Errorf("experience role %s %s = %q, want %q", role.id, check.label, got, check.want)
			}
		}
		technologyMarkups, techErr := findTestHTMLElementMarkups(markup, `data-role-technology`)
		if techErr != nil {
			return fmt.Errorf("experience role %s technologies: %w", role.id, techErr)
		}
		technologies := make([]string, 0, len(technologyMarkups))
		for _, technologyMarkup := range technologyMarkups {
			technology, textErr := testHTMLTextContent(technologyMarkup)
			if textErr != nil {
				return fmt.Errorf("experience role %s technology text: %w", role.id, textErr)
			}
			technologies = append(technologies, technology)
		}
		if !reflect.DeepEqual(technologies, role.technologies) {
			return fmt.Errorf("experience role %s technologies = %v, want %v", role.id, technologies, role.technologies)
		}
	}
	return nil
}

func requireNoFragmentTrail(t *testing.T, path, body string) {
	t.Helper()
	if strings.HasPrefix(path, "/skills/filtered") {
		if count := strings.Count(body, "data-signal-trail"); count != 1 {
			t.Errorf("GET %s fragment signal trail marker count = %d, want 1 card-field underlay", path, count)
		}
		if !strings.Contains(body, `class="signal-trail signal-trail-workbench skills-workbench-trail"`) {
			t.Errorf("GET %s fragment does not preserve the Skills card-field trail", path)
		}
		return
	}
	if count := strings.Count(body, "data-signal-trail"); count != 0 {
		t.Errorf("GET %s fragment signal trail marker count = %d, want 0", path, count)
	}
}

func assertRenderedPageShell(t *testing.T, path, body, bodyClass, pageMarker, shell string) {
	t.Helper()

	markers := []string{
		`<body class="` + bodyClass + `" data-shell="` + shell + `">`,
		`class="site-skip-link"`,
		`class="` + pageMarker,
		`/static/css/tailwind.css?v=20260818c`,
		`/static/js/main.js?v=20260810d`,
	}
	if count := strings.Count(body, "<h1"); count != 1 {
		t.Errorf("GET %s h1 count = %d, want 1", path, count)
	}
	if shell == "operator" {
		if !strings.Contains(body, ">Back to portfolio</a>") {
			t.Errorf("GET %s operator shell does not contain Back to portfolio", path)
		}
		if strings.Contains(body, `aria-label="Footer navigation"`) {
			t.Errorf("GET %s operator shell unexpectedly contains public footer navigation", path)
		}
	}
	for _, marker := range markers {
		if !strings.Contains(body, marker) {
			t.Errorf("GET %s body does not contain configured page marker %q", path, marker)
		}
	}

	seenIDs := make(map[string]struct{})
	for _, match := range renderedHTMLIDPattern.FindAllStringSubmatch(body, -1) {
		id := match[1]
		if _, exists := seenIDs[id]; exists {
			t.Errorf("GET %s renders duplicate id %q", path, id)
			continue
		}
		seenIDs[id] = struct{}{}
	}
}

func assertResponseMediaType(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()

	contentType := response.Header().Get("Content-Type")
	got, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("Content-Type %q is invalid: %v", contentType, err)
	}
	if got != want {
		t.Errorf("Content-Type media type = %q, want %q", got, want)
	}
}
