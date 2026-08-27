package main

import (
	"strings"
	"testing"
)

func TestValidateJSONStreamAcceptsUniqueObjects(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"JSONL without final newline": strings.Join([]string{
			`{"kind":"sample","health":{"status":200},"routes":[{"path":"/"},{"path":"/soccer"}]}`,
			`{"kind":"workflow","request_ids":["connect-1","add-1","sync-1"]}`,
		}, "\n"),
		"CRLF JSONL with repeated names in separate scopes": "{\"name\":\"outer\",\"nested\":{\"name\":\"inner\"}}\r\n{\"name\":\"second record\"}\r\n",
		"whitespace-separated objects":                      `{"kind":"sample"} {"kind":"workflow"}`,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := validateJSONStream(strings.NewReader(input)); err != nil {
				t.Fatalf("validateJSONStream() returned an error for valid JSONL: %v", err)
			}
		})
	}
}

func TestValidateJSONStreamRejectsDuplicateMembers(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"top level":           `{"kind":"sample","kind":"workflow"}`,
		"nested object":       `{"health":{"status":500,"status":200}}`,
		"object inside array": `{"routes":[{"path":"/private","path":"/"}]}`,
		"escaped equivalent":  `{"health":1,"he\u0061lth":2}`,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := validateJSONStream(strings.NewReader(input)); err == nil {
				t.Fatal("validateJSONStream() accepted a duplicate member")
			}
		})
	}
}

func TestValidateJSONStreamRejectsInvalidStreams(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"empty":           " \n\t",
		"top-level array": `[{"kind":"sample"}]`,
		"invalid JSON":    `{"kind":`,
		"invalid UTF-8":   string([]byte{'{', '"', 'k', '"', ':', '"', 0xff, '"', '}'}),
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := validateJSONStream(strings.NewReader(input)); err == nil {
				t.Fatal("validateJSONStream() accepted an invalid JSONL stream")
			}
		})
	}
}

func TestValidateJSONStreamErrorsDoNotExposeDuplicateContent(t *testing.T) {
	t.Parallel()

	const input = `{"health":{"oauth_token":"durable-secret"},"health":{"status":200}}`
	err := validateJSONStream(strings.NewReader(input))
	if err == nil {
		t.Fatal("validateJSONStream() accepted a duplicate member")
	}
	for _, forbidden := range []string{"health", "oauth_token", "durable-secret"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("validation error exposed forbidden record content %q", forbidden)
		}
	}
}
