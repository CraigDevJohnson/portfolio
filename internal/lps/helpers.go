package lps

import (
	"sort"
	"strconv"
	"strings"

	"portfolio/types"
)

// fullName joins non-empty trimmed name parts with spaces.
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

// firstNonEmptyString returns the first trimmed non-empty string.
func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// firstPositiveInt returns the first positive integer.
func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// intString returns the decimal string for a positive integer.
func intString(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}

// stringPointerValue returns a trimmed string pointer value.
func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// sortedUniqueIDs returns positive IDs in ascending order without duplicates.
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

func buildGameFacility(id int, name, address, city, state, zip string) *types.Facility {
	return types.NewFacilityDetails(id, name, address, city, state, zip)
}

func gameFacilityName(facility *types.Facility) string {
	if facility != nil && strings.TrimSpace(facility.Name) != "" {
		return strings.TrimSpace(facility.Name)
	}
	return ""
}
