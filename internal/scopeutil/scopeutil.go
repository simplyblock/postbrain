// Package scopeutil provides shared helpers for parsing scope strings.
package scopeutil

import (
	"errors"
	"strings"
)

// ErrEmptyScope is returned when an empty scope string is passed to ParseScopeString.
var ErrEmptyScope = errors.New("scope: empty scope string")

// ParseScopeString splits a scope string of the form "kind:external_id" into
// its kind and externalID components. Both parts must be non-empty after
// trimming whitespace. It returns an error if the string is empty, does not
// contain a colon separator, or has an empty kind or external_id.
func ParseScopeString(scope string) (kind, externalID string, err error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "", "", ErrEmptyScope
	}
	kind, externalID, found := strings.Cut(scope, ":")
	if !found {
		return "", "", errors.New("scope: missing ':' separator in scope string: " + scope)
	}
	kind = strings.TrimSpace(kind)
	externalID = strings.TrimSpace(externalID)
	if kind == "" {
		return "", "", errors.New("scope: empty kind in scope string: " + scope)
	}
	if externalID == "" {
		return "", "", errors.New("scope: empty external_id in scope string: " + scope)
	}
	return kind, externalID, nil
}
