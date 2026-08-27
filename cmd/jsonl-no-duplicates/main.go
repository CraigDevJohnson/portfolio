package main

import (
	"encoding/json/jsontext"
	"errors"
	"io"
	"os"
)

var (
	errDuplicateMember = errors.New("duplicate JSON object member")
	errInvalidJSON     = errors.New("invalid JSON stream")
)

func main() {
	if len(os.Args) != 1 {
		failValidation()
	}

	if err := validateJSONStream(os.Stdin); err != nil {
		failValidation()
	}
}

func failValidation() {
	_, _ = os.Stderr.WriteString("JSONL duplicate-member validation failed\n")
	os.Exit(1)
}

func validateJSONStream(input io.Reader) error {
	decoder := jsontext.NewDecoder(input)

	records := 0
	for {
		value, err := decoder.ReadValue()
		if errors.Is(err, io.EOF) {
			if records == 0 {
				return errInvalidJSON
			}
			return nil
		}
		if err != nil {
			if errors.Is(err, jsontext.ErrDuplicateName) {
				return errDuplicateMember
			}
			return errInvalidJSON
		}

		if value.Kind() != jsontext.KindBeginObject {
			return errInvalidJSON
		}
		records++
	}
}
