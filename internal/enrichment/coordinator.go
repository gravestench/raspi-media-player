package enrichment

import (
	"context"
	"errors"
	"net/url"
	"strings"
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
	images      *ImageCache
	slots       chan struct{}
}

func (c *Coordinator) WithImageCache(cache *ImageCache) *Coordinator { c.images = cache; return c }
func (c *Coordinator) WithMaxInflight(limit int) *Coordinator {
	if limit < 1 {
		limit = 1
	}
	c.slots = make(chan struct{}, limit)
	return c
}

func NewCoordinator(store *Store, ttl time.Duration, providers ...Provider) *Coordinator {
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	_ = store.Prune(context.Background(), time.Now())
	return &Coordinator{store: store, providers: providers, ttl: ttl, negativeTTL: 6 * time.Hour, inflight: make(map[string]struct{}), slots: make(chan struct{}, 2)}
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
	c.slots <- struct{}{}
	defer func() { <-c.slots }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	merged := Result{CacheKey: key, Hint: hint, Genres: []string{}, RelatedArtists: []ArtistSummary{}, Status: "ready"}
	found := false
	providerNames := []string{}
	type providerResult struct {
		index int
		value Result
		err   error
	}
	results := make(chan providerResult, len(c.providers))
	var wait sync.WaitGroup
	for index, provider := range c.providers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, err := provider.Lookup(ctx, hint)
			results <- providerResult{index: index, value: value, err: err}
		}()
	}
	wait.Wait()
	close(results)
	ordered := make([]providerResult, len(c.providers))
	for result := range results {
		ordered[result.index] = result
	}
	for index, result := range ordered {
		if result.err != nil {
			continue
		}
		found = true
		providerNames = append(providerNames, c.providers[index].Name())
		mergeResult(&merged, result.value)
	}
	if found {
		if merged.Image.URL != "" && c.images != nil && strings.Contains(merged.Provider, "wikimedia") {
			if cached, err := c.images.Cache(ctx, key, merged.Image); err == nil {
				merged.Image = cached
			}
		}
		merged.Provider = strings.Join(providerNames, ",")
		merged.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
		merged.ExpiresAt = time.Now().Add(c.ttl).UTC().Format(time.RFC3339Nano)
		_ = c.store.Put(ctx, merged)
		return
	}
	_ = c.store.Put(context.Background(), Result{CacheKey: key, Hint: hint, Status: "not_found", ErrorCode: "provider_not_found", FetchedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().Add(c.negativeTTL).UTC().Format(time.RFC3339Nano)})
}

func mergeResult(destination *Result, incoming Result) {
	if incoming.Hint.Artist != "" && incoming.Provider != "wikimedia" {
		destination.Hint.Artist = incoming.Hint.Artist
	}
	if destination.ArtistURL == "" && incoming.ArtistURL != "" {
		destination.ArtistURL = incoming.ArtistURL
	}
	if destination.Biography == "" {
		destination.Biography = incoming.Biography
	}
	destination.Genres = mergeStrings(destination.Genres, incoming.Genres, 12)
	for _, artist := range incoming.RelatedArtists {
		duplicate := false
		for _, existing := range destination.RelatedArtists {
			if strings.EqualFold(existing.Name, artist.Name) {
				duplicate = true
				break
			}
		}
		if !duplicate && artist.Name != "" && len(destination.RelatedArtists) < 12 {
			destination.RelatedArtists = append(destination.RelatedArtists, artist)
		}
	}
	if validImage(incoming.Image) && (destination.Image.URL == "" || incoming.Provider == "wikimedia") {
		destination.Image = incoming.Image
	}
}
func mergeStrings(current, incoming []string, limit int) []string {
	for _, value := range incoming {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		duplicate := false
		for _, existing := range current {
			if strings.EqualFold(existing, value) {
				duplicate = true
				break
			}
		}
		if !duplicate && len(current) < limit {
			current = append(current, value)
		}
	}
	return current
}
func validImage(image Image) bool {
	parsed, err := url.Parse(image.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || image.SourceURL == "" || image.Attribution == "" {
		return false
	}
	lower := strings.ToLower(image.URL)
	return !strings.Contains(lower, "2a96cbd8b46e442fc41c2b86b821562f") && !strings.Contains(lower, "default_artist")
}
