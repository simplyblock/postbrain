package social

import "context"

// UserInfo contains normalized social profile fields used by identity linking.
type UserInfo struct {
	ProviderID    string
	Email         string
	EmailVerified bool
	HostedDomain  string
	DisplayName   string
	AvatarURL     string
	RawProfile    []byte
}

// Provider defines the OAuth operations needed for social login flows.
type Provider interface {
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (*UserInfo, error)
}
