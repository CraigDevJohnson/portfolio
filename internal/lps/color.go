package lps

import "strings"

// approvedTeamColor maps LPS names to the closed set of colors with authored
// browser styles. Unrecognized upstream text never becomes a CSS value.
func approvedTeamColor(raw string) string {
	switch strings.ToLower(strings.Join(strings.Fields(raw), " ")) {
	case "red", "scarlet":
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
	case "gray", "grey", "silver":
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
