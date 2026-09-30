package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	// wcagNormalTextContrast is the WCAG 2.x AA minimum for normal-size text.
	wcagNormalTextContrast = 4.5
	// wcagNonTextContrast is the WCAG 2.x AA minimum for user interface
	// components and focus indicators.
	wcagNonTextContrast = 3.0
)

var (
	soccerThemeTokenPattern   = regexp.MustCompile(`(--[a-z0-9-]+)\s*:\s*([^;]+);`)
	soccerVarPattern          = regexp.MustCompile(`^var\((--[a-z0-9-]+)\)$`)
	soccerHexPattern          = regexp.MustCompile(`^#([0-9a-fA-F]{6})$`)
	soccerMixPattern          = regexp.MustCompile(`^color-mix\(in srgb,\s*(.+?)\s+([0-9.]+)%,\s*(.+)\)$`)
	soccerOverlayPattern      = regexp.MustCompile(`^color-mix\(in srgb,\s*(var\(--[a-z0-9-]+\))\s+([0-9.]+)%,\s*transparent\)$`)
	soccerOutlineColorPattern = regexp.MustCompile(`var\(--[a-z0-9-]+\)`)
	soccerVarRefPattern       = regexp.MustCompile(`var\((--[a-z0-9-]+)\)`)
)

// TestSoccerTeamColorStylesheetKeepsRowsLegible checks the stylesheet contract
// behind every color the real fetch route emits: each color paints a row half;
// row text keeps WCAG AA text contrast over the translucent cell overlay on
// each half; the checked selection control and its keyboard focus outline keep
// 3:1 non-text contrast there; and no width breakpoint repaints those surfaces.
// It computes declared token values only. Rendered narrow, forced-colors, and
// focus layouts need a browser check.
func TestSoccerTeamColorStylesheetKeepsRowsLegible(t *testing.T) {
	emitted := soccerEmittedTeamColors(t)

	rules, err := collectExperienceCSSRules(readTask2Artifact(t, "cmd", "web", "tailwind", "soccer.css"), 0, false, false)
	if err != nil {
		t.Fatalf("parse Soccer CSS: %v", err)
	}
	theme := soccerPageTokens(t)

	row := soccerEffectiveDeclarations(rules, ".soccer-match-row", 0, false)
	if want := "linear-gradient(90deg, var(--soccer-home-paint) 0 50%, var(--soccer-away-paint) 50% 100%)"; !task2CSSValueEqual(row["background"], want) {
		t.Fatalf(".soccer-match-row background = %q, want home and away halves %q", row["background"], want)
	}

	paints := make(map[string]final10RGB)
	for color := range emitted {
		for _, side := range []string{"home", "away"} {
			selector := fmt.Sprintf(".soccer-match-row[data-%s-color='%s']", side, color)
			value := soccerEffectiveDeclarations(rules, selector, 0, false)["--soccer-"+side+"-paint"]
			rgb, resolveErr := soccerResolveColor(theme, value)
			if resolveErr != nil {
				t.Errorf("emitted %s color %q is not painted by %s: %v", side, color, selector, resolveErr)
				continue
			}
			paints[side+" "+color] = rgb
		}
	}
	for _, color := range []string{"white", "yellow"} {
		if !emitted[color] {
			t.Fatalf("fetch route did not emit the light %q paint this contract must cover", color)
		}
	}

	// surfaces returns what a cell shows over each painted half: its
	// translucent overlay mixed with the team paint beneath.
	surfaces := func(cell string) map[string]final10RGB {
		t.Helper()
		background := soccerEffectiveDeclarations(rules, cell, 0, false)["background"]
		match := soccerOverlayPattern.FindStringSubmatch(task2CanonicalCSS(background))
		if match == nil {
			t.Fatalf("%s background = %q, want a translucent overlay above the team halves", cell, background)
		}
		overlay, resolveErr := soccerResolveColor(theme, match[1])
		if resolveErr != nil {
			t.Fatalf("%s overlay: %v", cell, resolveErr)
		}
		amount, _ := strconv.ParseFloat(match[2], 64)
		mixed := make(map[string]final10RGB, len(paints))
		for paint, rgb := range paints {
			mixed[paint] = final10Mix(overlay, rgb, amount/100)
		}
		return mixed
	}
	requireContrast := func(label, value string, over map[string]final10RGB, minimum float64) {
		t.Helper()
		foreground, resolveErr := soccerResolveColor(theme, value)
		if resolveErr != nil {
			t.Fatalf("%s color %q: %v", label, value, resolveErr)
		}
		for paint, surface := range over {
			if ratio := final10Contrast(foreground, surface); ratio < minimum {
				t.Errorf("%s on the %s half has contrast %.2f, want >= %.1f", label, paint, ratio, minimum)
			}
		}
	}

	detail := surfaces(".soccer-match-detail")
	for _, text := range []string{".soccer-match-detail", ".soccer-match-detail::before", ".soccer-match-datetime", ".soccer-matchup strong", ".soccer-match-versus"} {
		requireContrast(text+" text", soccerEffectiveDeclarations(rules, text, 0, false)["color"], detail, wcagNormalTextContrast)
	}

	// The select cell holds only the checkbox, so it is held to the WCAG 3:1
	// non-text minimum: the checked fill (accent-color) and the keyboard focus
	// outline drawn around the checkbox inside the cell.
	selectCell := surfaces(".soccer-match-select")
	requireContrast("checked checkbox accent", soccerEffectiveDeclarations(rules, ".soccer-stage input[type='checkbox']", 0, false)["accent-color"], selectCell, wcagNonTextContrast)
	requireContrast("checkbox focus outline", soccerFocusOutlineColor(t), selectCell, wcagNonTextContrast)

	// Narrow and wide layouts reuse these surfaces: no width breakpoint may
	// repaint a row, a cell, or its text.
	colorProperties := []string{"background", "background-color", "background-image", "color", "accent-color", "outline", "outline-color", "--soccer-home-paint", "--soccer-away-paint"}
	for _, rule := range rules {
		if rule.minWidthRem <= 0 || rule.forcedColors {
			continue
		}
		for _, selector := range []string{".soccer-match-row", ".soccer-match-select", ".soccer-match-detail", ".soccer-match-detail::before", ".soccer-match-datetime", ".soccer-matchup strong", ".soccer-match-versus", ".soccer-stage input[type='checkbox']"} {
			if !task2SelectorListContains(strings.ReplaceAll(rule.selector, `"`, `'`), selector) {
				continue
			}
			for _, property := range colorProperties {
				if value, ok := rule.declarations[property]; ok {
					t.Errorf("width >= %.0frem rule %q repaints %s with %s: %s", rule.minWidthRem, rule.selector, selector, property, value)
				}
			}
		}
	}

	// Forced-colors layouts must replace the halves with system colors. This
	// checks the declarations only; rendering is a browser check.
	for selector, want := range map[string]map[string]string{
		".soccer-match-row":    {"background": "Canvas", "color": "CanvasText"},
		".soccer-match-select": {"background": "Canvas", "color": "CanvasText"},
		".soccer-match-detail": {"background": "Canvas", "color": "CanvasText"},
	} {
		if !experienceHasEffectiveRule(rules, selector, 0, true, false, want) {
			t.Errorf("forced-colors %s lacks %v", selector, want)
		}
	}
}

// soccerPageTokens returns the custom properties in effect inside the Soccer
// page. The .page-kit-page wrapper re-points theme tokens. As in CSS, a var()
// in its declarations resolves to a property declared on the same wrapper, or
// else to the value inherited from the root, kept under a "--root" alias.
func soccerPageTokens(t *testing.T) map[string]string {
	t.Helper()
	root := soccerThemeTokens(readTask2Artifact(t, "cmd", "web", "tailwind", "theme.css"))
	tokens := make(map[string]string, 2*len(root))
	for name, value := range root {
		tokens[name] = value
		tokens["--root"+name] = soccerVarRefPattern.ReplaceAllString(value, "var(--root$1)")
	}
	blocks, err := parseTask2CSSBlocks(readTask2Artifact(t, "cmd", "web", "tailwind", "components.css"))
	if err != nil {
		t.Fatalf("parse components CSS: %v", err)
	}
	page := make(map[string]string)
	for _, block := range blocks {
		if !task2SelectorListContains(block.header, ".page-kit-page") {
			continue
		}
		for property, value := range task2Declarations(block.body) {
			if strings.HasPrefix(property, "--") {
				page[property] = strings.TrimSpace(value)
			}
		}
	}
	for property, value := range page {
		tokens[property] = soccerVarRefPattern.ReplaceAllStringFunc(value, func(ref string) string {
			name := soccerVarRefPattern.FindStringSubmatch(ref)[1]
			if _, declared := page[name]; declared {
				return ref
			}
			return "var(--root" + name + ")"
		})
	}
	return tokens
}

// soccerFocusOutlineColor returns the color of the base :focus-visible
// outline, which the Soccer page does not override outside forced colors.
func soccerFocusOutlineColor(t *testing.T) string {
	t.Helper()
	blocks, err := parseTask2CSSBlocks(readTask2Artifact(t, "cmd", "web", "tailwind", "base.css"))
	if err != nil {
		t.Fatalf("parse base CSS: %v", err)
	}
	outline := ""
	for _, block := range blocks {
		if task2SelectorListContains(block.header, ":focus-visible") {
			if value, ok := task2Declarations(block.body)["outline"]; ok {
				outline = value
			}
		}
	}
	color := soccerOutlineColorPattern.FindString(outline)
	if color == "" {
		t.Fatalf(":focus-visible outline = %q, want a var() color", outline)
	}
	return color
}

// soccerEmittedTeamColors collects every data-*-color value the /soccer/fetch
// route renders for recognized, unrecognized, and missing LPS team colors.
func soccerEmittedTeamColors(t *testing.T) map[string]bool {
	t.Helper()
	app := newTestApp(t)
	payloads := make(map[string]string)
	var teamIDs []string
	addTeam := func(teamID, gameID int, color string) {
		payloads[fmt.Sprintf("/teams/%d", teamID)] = fmt.Sprintf(
			`{"team":{"UTeamID":%d,"team_name":"Team %d","Color":%q},"games":[{"UGameID":%d,"SchedGameDateTime":"{future}","UTeam1":%d,"UTeam2":%d,"home_team":{"UTeamID":%d,"team_name":"Team %d"},"visitor_team":{"UTeamID":%d,"team_name":"Opponent %d"}}]}`,
			teamID, teamID, color, gameID, teamID, teamID+50000, teamID, teamID, teamID+50000, teamID,
		)
		teamIDs = append(teamIDs, strconv.Itoa(teamID))
	}
	recognized := []string{
		"Red", "BLUE", "navy blue", "Forest Green", "yellow", "Gold", "orange", "Purple", "pink", "Teal",
		"maroon", "black", "white", "Grey", "silver", "Royal Blue", "Sky Blue", "Kelly Green",
	}
	unrecognized := []string{"Magenta", "#ff0000", "rgb(1, 2, 3)"}
	for i, color := range append(recognized, unrecognized...) {
		addTeam(1000+i, 5000+i, color)
	}
	for i := range 16 {
		addTeam(2000+i, 6000+i, "")
	}
	server := newFakeLPSTeams(t, payloads)
	app.Config.LPSAPIBaseURL = server.URL
	mux, _ := buildMux(app, app.Logger, false)

	rows, _ := fetchSoccerMatchRows(t, mux, teamIDs...)
	if len(rows) != len(teamIDs) {
		t.Fatalf("rendered rows for %d games, want %d", len(rows), len(teamIDs))
	}
	emitted := make(map[string]bool)
	for id, nodes := range rows {
		for _, node := range nodes {
			for _, attr := range []string{"data-home-color", "data-away-color"} {
				color := htmlAttr(node, attr)
				if color == "" {
					t.Fatalf("game %s row lacks %s", id, attr)
				}
				emitted[color] = true
			}
		}
	}
	return emitted
}

func soccerThemeTokens(css string) map[string]string {
	tokens := make(map[string]string)
	for _, match := range soccerThemeTokenPattern.FindAllStringSubmatch(css, -1) {
		if _, seen := tokens[match[1]]; !seen {
			tokens[match[1]] = strings.TrimSpace(match[2])
		}
	}
	return tokens
}

// soccerResolveColor resolves hex, var(), and opaque srgb color-mix() values
// against theme tokens.
func soccerResolveColor(tokens map[string]string, value string) (final10RGB, error) {
	value = task2CanonicalCSS(value)
	for depth := 0; depth < 16; depth++ {
		if match := soccerVarPattern.FindStringSubmatch(value); match != nil {
			next, ok := tokens[match[1]]
			if !ok {
				return final10RGB{}, fmt.Errorf("undefined theme token %s", match[1])
			}
			value = task2CanonicalCSS(next)
			continue
		}
		if match := soccerHexPattern.FindStringSubmatch(value); match != nil {
			parsed, err := strconv.ParseUint(match[1], 16, 32)
			if err != nil {
				return final10RGB{}, err
			}
			return final10RGB{red: float64(parsed >> 16 & 0xff), green: float64(parsed >> 8 & 0xff), blue: float64(parsed & 0xff)}, nil
		}
		if match := soccerMixPattern.FindStringSubmatch(value); match != nil && !strings.Contains(match[3], "transparent") {
			first, err := soccerResolveColor(tokens, match[1])
			if err != nil {
				return final10RGB{}, err
			}
			second, err := soccerResolveColor(tokens, match[3])
			if err != nil {
				return final10RGB{}, err
			}
			amount, err := strconv.ParseFloat(match[2], 64)
			if err != nil {
				return final10RGB{}, err
			}
			return final10Mix(first, second, amount/100), nil
		}
		return final10RGB{}, fmt.Errorf("unsupported color value %q", value)
	}
	return final10RGB{}, fmt.Errorf("color value %q nests too deeply", value)
}

func soccerEffectiveDeclarations(rules []experienceCSSRule, selector string, widthRem float64, forced bool) map[string]string {
	want := strings.ReplaceAll(task2CanonicalCSS(selector), `"`, `'`)
	effective := make(map[string]string)
	for _, rule := range rules {
		if rule.minWidthRem > widthRem || rule.forcedColors && !forced || rule.reducedMotion {
			continue
		}
		for _, candidate := range task2SplitTopLevel(rule.selector, ',') {
			if strings.ReplaceAll(task2CanonicalCSS(candidate), `"`, `'`) == want {
				for property, value := range rule.declarations {
					effective[property] = value
				}
				break
			}
		}
	}
	return effective
}
