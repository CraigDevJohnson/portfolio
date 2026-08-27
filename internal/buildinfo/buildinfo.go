package buildinfo

import "strings"

var revision = "development"

func Revision() string {
	value := strings.TrimSpace(revision)
	if value == "" {
		return "development"
	}
	return value
}
