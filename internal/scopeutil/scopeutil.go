// Package scopeutil provides shared helpers for parsing scope strings.
package scopeutil

import (
	"errors"
	"strings"
)

// ErrEmptyScope is returned when an empty scope string is passed to ParseScopeString.
var ErrEmptyScope = errors.New("scope: empty scope string")

// ParseScopeString splits a scope string of the form "kind:external_id" into
// its kind and externalID components. It returns an error if the string is
// empty or does not contain a colon separator.
func ParseScopeString(scope string) (kind, externalID string, err error) {
	if scope == "" {
		return "", "", ErrEmptyScope
	}
	kind, externalID, found := strings.Cut(scope, ":")
	if !found {
		return "", "", errors.New("scope: missing ':' separator in scope string: " + scope)
	}
	return kind, externalID, nil
}
