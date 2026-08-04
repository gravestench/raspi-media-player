package enrichment

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Coordinator struct {
	store       *Store
	providers   []Provider
	ttl         time.Duration
	negativeTTL time.Duration
	mu          sync.Mutex
	inflight    map[string]struct{}
}

func NewCoordinator(store *Store, ttl time.Duration, providers ...Provider) *Coordinator {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &Coordinator{store: store, providers: providers, ttl: ttl, negativeTTL: 6 * time.Hour, inflight: make(map[string]struct{})}
}

func (c *Coordinator) ObserveTitle(ctx context.Context, title string) { _, _ = c.Lookup(ctx, title) }

func (c *Coordinator) Lookup(ctx context.Context, title string) (Result, error) {
	hint := ParseTitle(title)
	if hint.Artist == "" {
		return Result{Hint: hint, Status: "not_found"}, ErrProviderNotFound
	}
	key := Key(hint)
	cached, err := c.store.Get(ctx, key)
	if err == nil {
		if expires, parseErr := time.Parse(time.RFC3339Nano, cached.ExpiresAt); parseErr == nil && time.Now().Before(expires) {
			return cached, nil
		}
	} else if !errors.Is(err, ErrNotFound) {
		return Result{}, err
	}
	pending := Result{CacheKey: key, Hint: hint, Status: "pending", ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339Nano)}
	if err := c.store.Put(ctx, pending); err != nil {
		return Result{}, err
	}
	c.mu.Lock()
	if _, exists := c.inflight[key]; exists {
		c.mu.Unlock()
		return pending, nil
	}
	c.inflight[key] = struct{}{}
	c.mu.Unlock()
	go c.fetch(key, hint)
	return pending, nil
}

func (c *Coordinator) fetch(key string, hint TrackHint) {
	defer func() { c.mu.Lock(); delete(c.inflight, key); c.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	for _, provider := range c.providers {
		result, err := provider.Lookup(ctx, hint)
		if err != nil {
			continue
		}
		result.CacheKey = key
		result.Hint.RawTitle = hint.RawTitle
		result.Status = "ready"
		result.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
		result.ExpiresAt = time.Now().Add(c.ttl).UTC().Format(time.RFC3339Nano)
		_ = c.store.Put(ctx, result)
		return
	}
	_ = c.store.Put(context.Background(), Result{CacheKey: key, Hint: hint, Status: "not_found", ErrorCode: "provider_not_found", FetchedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().Add(c.negativeTTL).UTC().Format(time.RFC3339Nano)})
}
