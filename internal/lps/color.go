package lps

import "strings"

// approvedTeamColor maps LPS names to the closed set of colors with authored
// browser styles. Unrecognized upstream text never becomes a CSS value.
//
// Whole phrases are matched first, so a modifier that changes the hue (such as
// "dark blue" to navy) stays explicit. Otherwise a modified name such as
// "Kelly Green" or "Sky Blue" is recognized by its final word, which must
// itself be a known color name.
func approvedTeamColor(raw string) string {
	words := strings.Fields(strings.ToLower(raw))
	if len(words) == 0 {
		return ""
	}
	if color := approvedTeamColorPhrase(strings.Join(words, " ")); color != "" {
		return color
	}
	return approvedTeamColorPhrase(words[len(words)-1])
}

func approvedTeamColorPhrase(phrase string) string {
	switch phrase {
	case "red", "scarlet", "crimson", "cardinal":
		return "red"
	case "blue", "royal blue", "light blue":
		return "blue"
	case "navy", "navy blue", "dark blue":
		return "navy"
	case "green", "forest green", "lime green":
		return "green"
	case "yellow":
		return "yellow"
	case "gold":
		return "gold"
	case "orange":
		return "orange"
	case "purple", "violet":
		return "purple"
	case "pink":
		return "pink"
	case "teal", "turquoise":
		return "teal"
	case "maroon", "burgundy":
		return "maroon"
	case "black":
		return "black"
	case "white":
		return "white"
	case "gray", "grey", "silver", "charcoal":
		return "gray"
	default:
		return ""
	}
}

func firstApprovedTeamColor(values ...string) string {
	for _, value := range values {
		if color := approvedTeamColor(value); color != "" {
			return color
		}
	}
	return ""
}
