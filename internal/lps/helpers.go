package lps

import (
	"sort"
	"strconv"
	"strings"
)

// FullName joins non-empty trimmed name parts with spaces.
func FullName(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, " ")
}

// FirstNonEmptyString returns the first trimmed non-empty string.
func FirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// FirstPositiveInt returns the first positive integer.
func FirstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// IntString returns the decimal string for a positive integer.
func IntString(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}

// StringPointerValue returns a trimmed string pointer value.
func StringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// SortedUniqueIDs returns positive IDs in ascending order without duplicates.
func SortedUniqueIDs(values []int) []int {
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

// NonEmptyStrings returns trimmed non-empty strings.
func NonEmptyStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}
