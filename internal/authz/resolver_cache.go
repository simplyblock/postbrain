package authz

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type cacheKey struct {
	principalID uuid.UUID
	scopeID     uuid.UUID
}

type cacheEntry struct {
	perms     PermissionSet
	expiresAt time.Time
}

// CachedResolver wraps a Resolver with a short-lived in-process cache keyed on
// (principalID, scopeID) to avoid repeated DB round-trips per request.
type CachedResolver struct {
	inner Resolver
	ttl   time.Duration

	mu    sync.Mutex
	cache map[cacheKey]cacheEntry
}

// NewCachedResolver wraps r with a cache whose entries expire after ttl.
func NewCachedResolver(r Resolver, ttl time.Duration) *CachedResolver {
	return &CachedResolver{
		inner: r,
		ttl:   ttl,
		cache: make(map[cacheKey]cacheEntry),
	}
}

// EffectivePermissions returns cached permissions if fresh, otherwise delegates
// to the underlying resolver and caches the result.
func (c *CachedResolver) EffectivePermissions(ctx context.Context, principalID, scopeID uuid.UUID) (PermissionSet, error) {
	key := cacheKey{principalID, scopeID}

	c.mu.Lock()
	if entry, ok := c.cache[key]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.Unlock()
		return entry.perms, nil
	}
	c.mu.Unlock()

	perms, err := c.inner.EffectivePermissions(ctx, principalID, scopeID)
	if err != nil {
		return EmptyPermissionSet(), err
	}

	c.mu.Lock()
	c.cache[key] = cacheEntry{perms: perms, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()

	return perms, nil
}

// HasPermission returns true if the principal holds perm on scopeID, using
// cached permissions where available.
func (c *CachedResolver) HasPermission(ctx context.Context, principalID, scopeID uuid.UUID, perm Permission) (bool, error) {
	perms, err := c.EffectivePermissions(ctx, principalID, scopeID)
	if err != nil {
		return false, err
	}
	return perms.Contains(perm), nil
}

// Invalidate removes all cached entries for the given (principalID, scopeID) pair.
func (c *CachedResolver) Invalidate(principalID, scopeID uuid.UUID) {
	c.mu.Lock()
	delete(c.cache, cacheKey{principalID, scopeID})
	c.mu.Unlock()
}

// InvalidateScope removes all cached entries for a given scopeID across all principals.
func (c *CachedResolver) InvalidateScope(scopeID uuid.UUID) {
	c.mu.Lock()
	for k := range c.cache {
		if k.scopeID == scopeID {
			delete(c.cache, k)
		}
	}
	c.mu.Unlock()
}
