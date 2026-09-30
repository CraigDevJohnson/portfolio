package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// wcagNormalTextContrast is the WCAG 2.x AA minimum for normal-size text.
const wcagNormalTextContrast = 4.5

var (
	soccerThemeTokenPattern = regexp.MustCompile(`(--[a-z0-9-]+)\s*:\s*([^;]+);`)
	soccerVarPattern        = regexp.MustCompile(`^var\((--[a-z0-9-]+)\)$`)
	soccerHexPattern        = regexp.MustCompile(`^#([0-9a-fA-F]{6})$`)
	soccerMixPattern        = regexp.MustCompile(`^color-mix\(in srgb,\s*(.+?)\s+([0-9.]+)%,\s*(.+)\)$`)
	soccerOverlayPattern    = regexp.MustCompile(`^color-mix\(in srgb,\s*(var\(--[a-z0-9-]+\))\s+([0-9.]+)%,\s*transparent\)$`)
)

// TestSoccerTeamColorsArePaintedWithLegibleRowText checks that every color the
// real fetch route emits has a painted half in the stylesheet, and that row
// text keeps WCAG AA contrast on each half at every layout width.
func TestSoccerTeamColorsArePaintedWithLegibleRowText(t *testing.T) {
	emitted := soccerEmittedTeamColors(t)

	theme := soccerThemeTokens(readTask2Artifact(t, "cmd", "web", "tailwind", "theme.css"))
	rules, err := collectExperienceCSSRules(readTask2Artifact(t, "cmd", "web", "tailwind", "soccer.css"), 0, false, false)
	if err != nil {
		t.Fatalf("parse Soccer CSS: %v", err)
	}

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

	// Each translucent cell sits above both halves; its text must stay legible.
	cellText := map[string][]string{
		".soccer-match-select": {".soccer-match-select"},
		".soccer-match-detail": {".soccer-match-detail", ".soccer-match-detail::before", ".soccer-match-datetime", ".soccer-matchup strong", ".soccer-match-versus"},
	}
	for _, width := range []float64{0, 48, 70} {
		for cell, texts := range cellText {
			background := soccerEffectiveDeclarations(rules, cell, width, false)["background"]
			match := soccerOverlayPattern.FindStringSubmatch(task2CanonicalCSS(background))
			if match == nil {
				t.Fatalf("%s at %.0frem background = %q, want a translucent overlay above the team halves", cell, width, background)
			}
			overlay, resolveErr := soccerResolveColor(theme, match[1])
			if resolveErr != nil {
				t.Fatalf("%s overlay: %v", cell, resolveErr)
			}
			amount, _ := strconv.ParseFloat(match[2], 64)
			for _, text := range texts {
				value := soccerEffectiveDeclarations(rules, text, width, false)["color"]
				foreground, resolveErr := soccerResolveColor(theme, value)
				if resolveErr != nil {
					t.Fatalf("%s color %q: %v", text, value, resolveErr)
				}
				for paint, rgb := range paints {
					surface := final10Mix(overlay, rgb, amount/100)
					if ratio := final10Contrast(foreground, surface); ratio < wcagNormalTextContrast {
						t.Errorf("%s text on the %s half at %.0frem has contrast %.2f, want >= %.1f", text, paint, width, ratio, wcagNormalTextContrast)
					}
				}
			}
		}
	}

	// High-contrast (forced colors) layouts drop the halves for system colors.
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
	for i, color := range []string{
		"Red", "BLUE", "navy blue", "Forest Green", "yellow", "Gold", "orange", "Purple", "pink", "Teal",
		"maroon", "black", "white", "Grey", "silver", "Royal Blue", "Sky Blue", "Magenta", "#ff0000", "rgb(1, 2, 3)",
	} {
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
