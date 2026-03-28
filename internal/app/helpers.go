// General-purpose helpers: name formatting, ID parsing, IP detection, and proxy trust.
package app

import (
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	internalhttpx "portfolio/internal/httpx"
)

func fullName(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, " ")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func intString(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func sortedUniqueIDs(values []int) []int {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[int]struct{}, len(values))
	normalized := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Ints(normalized)
	return normalized
}

func nonEmptyStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func parseSelectedIDs(form url.Values) map[string]struct{} {
	selectedIDs := make(map[string]struct{})
	for _, id := range form["selected"] {
		id = strings.TrimSpace(id)
		if id != "" {
			selectedIDs[id] = struct{}{}
		}
	}
	return selectedIDs
}

func parsePlayerIDs(values []string) []int {
	seen := make(map[int]struct{})
	playerIDs := make([]int, 0, len(values))
	for _, value := range values {
		playerID, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || playerID <= 0 {
			continue
		}
		if _, exists := seen[playerID]; exists {
			continue
		}
		seen[playerID] = struct{}{}
		playerIDs = append(playerIDs, playerID)
	}
	return playerIDs
}

func splitDelimitedValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
}

func parseTeamIDs(raw string) []int {
	return parsePlayerIDs(splitDelimitedValues(raw))
}

func clientIP(r *http.Request) string {
	return internalhttpx.ClientIP(r)
}

func forwardedClientIP(r *http.Request) (string, bool) {
	return internalhttpx.ForwardedClientIP(r)
}

func remoteAddrIP(remoteAddr string) net.IP {
	return internalhttpx.RemoteAddrIP(remoteAddr)
}

func isTrustedProxyIP(ip net.IP) bool {
	return internalhttpx.IsTrustedProxyIP(ip)
}

func isValidIP(value string) bool {
	return internalhttpx.IsValidIP(value)
}
