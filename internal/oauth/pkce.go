package oauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// GenerateChallenge computes an RFC 7636 S256 challenge from a verifier.
func GenerateChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// VerifyS256 compares verifier-derived S256 challenge to the provided challenge.
func VerifyS256(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	derived := GenerateChallenge(verifier)
	if len(derived) != len(challenge) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(derived), []byte(challenge)) == 1
}
