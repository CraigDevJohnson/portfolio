package app

import (
	"fmt"
	"regexp"
	"strings"
)

// These helpers intentionally recognize only selector forms authored by the
// protected contracts. Unknown functional syntax that names a target fails.
func final10RuleTargets(selector string, target *final10Target) (bool, error) {
	branches, err := final10ExpandSelectorList(selector)
	if err != nil {
		return false, err
	}
	for _, branch := range branches {
		if final10PseudoElementSelector(branch) {
			continue
		}
		positive, negatives, stripErr := final10StripNot(branch)
		if stripErr != nil {
			return false, stripErr
		}
		matched := false
		for _, alias := range target.aliases {
			matched = matched || final10SelectorContainsAlias(positive, alias)
		}
		for _, excluded := range target.excluded {
			if final10SelectorContainsAlias(positive, excluded) {
				matched = false
			}
		}
		if !matched {
			continue
		}
		if regexp.MustCompile(`(?i):[-a-z]+\(`).MatchString(positive) {
			return false, fmt.Errorf("unsupported protected selector %q", branch)
		}
		excludedByNot := false
		for _, negative := range negatives {
			excluded, excludeErr := final10NegativeExcludesTarget(negative, target)
			if excludeErr != nil {
				return false, excludeErr
			}
			excludedByNot = excludedByNot || excluded
		}
		if !excludedByNot {
			return true, nil
		}
	}
	return false, nil
}

func final10ExpandSelectorList(selector string) ([]string, error) {
	var expanded []string
	for _, branch := range task2SplitTopLevel(selector, ',') {
		values, err := final10ExpandFunctionalSelector(branch, 0)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, values...)
	}
	return expanded, nil
}

func final10ExpandFunctionalSelector(selector string, depth int) ([]string, error) {
	if depth > 3 {
		return nil, fmt.Errorf("unsupported nested selector function %q", selector)
	}
	lower := strings.ToLower(selector)
	index := -1
	for _, name := range []string{":is(", ":where("} {
		from := 0
		for {
			found := strings.Index(lower[from:], name)
			if found < 0 {
				break
			}
			found += from
			if !final10IndexInsideNot(selector, found) && (index < 0 || found < index) {
				index = found
				break
			}
			from = found + len(name)
		}
	}
	if index < 0 {
		return []string{selector}, nil
	}
	opening := strings.IndexByte(selector[index:], '(') + index
	closing := final10MatchingParenthesis(selector, opening)
	if closing < 0 {
		return nil, fmt.Errorf("unclosed selector function in %q", selector)
	}
	var expanded []string
	for _, argument := range task2SplitTopLevel(selector[opening+1:closing], ',') {
		replaced := selector[:index] + argument + selector[closing+1:]
		values, err := final10ExpandFunctionalSelector(replaced, depth+1)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, values...)
	}
	return expanded, nil
}

func final10IndexInsideNot(selector string, index int) bool {
	lower := strings.ToLower(selector)
	from := 0
	for from < index {
		found := strings.Index(lower[from:index], ":not(")
		if found < 0 {
			return false
		}
		found += from
		opening := found + len(":not")
		if closing := final10MatchingParenthesis(selector, opening); closing >= index {
			return true
		}
		from = found + len(":not(")
	}
	return false
}

func final10MatchingParenthesis(value string, opening int) int {
	depth := 1
	for index := opening + 1; index < len(value); index++ {
		if value[index] == '\'' || value[index] == '"' {
			next, err := task2SkipCSSString(value, index)
			if err != nil {
				return -1
			}
			index = next - 1
			continue
		}
		switch value[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func final10StripNot(selector string) (string, []string, error) {
	var negatives []string
	for {
		index := strings.Index(strings.ToLower(selector), ":not(")
		if index < 0 {
			return selector, negatives, nil
		}
		opening := index + len(":not")
		closing := final10MatchingParenthesis(selector, opening)
		if closing < 0 {
			return "", nil, fmt.Errorf("unclosed :not() in %q", selector)
		}
		negatives = append(negatives, task2SplitTopLevel(selector[opening+1:closing], ',')...)
		selector = selector[:index] + selector[closing+1:]
	}
}

func final10NegativeExcludesTarget(negative string, target *final10Target) (bool, error) {
	alternatives, err := final10ExpandSelectorList(negative)
	if err != nil {
		return false, err
	}
	for _, alternative := range alternatives {
		normalized := final10NormalizeSelector(alternative)
		for _, guaranteed := range target.guaranteed {
			identity := final10NormalizeSelector(guaranteed)
			if normalized == identity {
				return true, nil
			}
			// Repeating the same simple selector is equivalent, while any
			// remaining compound requirement keeps the target applicable.
			if !strings.ContainsAny(identity, " >+~") && strings.Contains(normalized, identity) {
				remaining := strings.ReplaceAll(normalized, identity, "")
				remaining = strings.TrimPrefix(remaining, "*")
				for _, element := range target.types {
					remaining = strings.TrimPrefix(remaining, strings.ToLower(element))
				}
				if remaining == "" {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func final10PseudoElementSelector(selector string) bool {
	return strings.Contains(selector, "::") || regexp.MustCompile(`(?i):(before|after|first-line|first-letter|marker|placeholder)(?:[^-a-z0-9_]|$)`).MatchString(selector)
}

func final10SelectorContainsAlias(selector, alias string) bool {
	if strings.ContainsAny(alias, " >+~") {
		return strings.Contains(final10NormalizeSelector(selector), final10NormalizeSelector(alias))
	}
	subject := final10SelectorSubject(selector)
	if strings.HasPrefix(alias, ".") {
		return styleSelectorHasExactClass(subject, strings.TrimPrefix(alias, "."))
	}
	if strings.HasPrefix(alias, "#") {
		return skillsSelectorHasExactID(subject, strings.TrimPrefix(alias, "#"))
	}
	return strings.Contains(final10NormalizeSelector(subject), final10NormalizeSelector(alias))
}

func final10SelectorSubject(selector string) string {
	start := 0
	parentheses, brackets := 0, 0
	for index := 0; index < len(selector); index++ {
		if selector[index] == '\'' || selector[index] == '"' {
			next, err := task2SkipCSSString(selector, index)
			if err != nil {
				return selector
			}
			index = next - 1
			continue
		}
		switch selector[index] {
		case '(':
			parentheses++
		case ')':
			parentheses--
		case '[':
			brackets++
		case ']':
			brackets--
		case ' ', '\t', '\n', '\r', '>', '+', '~':
			if parentheses == 0 && brackets == 0 {
				start = index + 1
			}
		}
	}
	return strings.TrimSpace(selector[start:])
}

func final10NormalizeSelector(selector string) string {
	selector = task2CanonicalCSS(selector)
	return strings.NewReplacer(`"`, "", `'`, "", " > ", ">", " + ", "+", " ~ ", "~").Replace(selector)
}

// Kept here because the Soccer hint contract shares the same exact-ID forms.
func skillsSelectorHasExactID(selector, id string) bool {
	selector = task2CanonicalCSS(selector)
	if regexp.MustCompile(`#` + regexp.QuoteMeta(id) + `(?:[^a-zA-Z0-9_-]|$)`).MatchString(selector) {
		return true
	}
	for _, attribute := range []string{`[id="` + id + `"]`, `[id='` + id + `']`, `[id=` + id + `]`} {
		if strings.Contains(selector, attribute) {
			return true
		}
	}
	return false
}
