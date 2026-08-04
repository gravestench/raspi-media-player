package source

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid source")
	ErrUnsupported = errors.New("unsupported source")
	ErrUnavailable = errors.New("source unavailable")
)

type Playable struct {
	Kind        string
	OriginalURL string
	PlaybackURL string
	Title       string
}

type Adapter interface {
	Kind() string
	Match(string) bool
	Resolve(context.Context, string) (Playable, error)
}

type Resolver interface {
	Classify(string) (string, error)
	Resolve(context.Context, string, string) (Playable, error)
}

type cached struct {
	value     Playable
	expiresAt time.Time
}

type Registry struct {
	adapters map[string]Adapter
	order    []Adapter
	timeout  time.Duration
	cacheTTL time.Duration
	mu       sync.Mutex
	cache    map[string]cached
}

func NewRegistry(timeout, cacheTTL time.Duration, adapters ...Adapter) *Registry {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	registry := &Registry{adapters: make(map[string]Adapter), timeout: timeout, cacheTTL: cacheTTL, cache: make(map[string]cached)}
	for _, adapter := range adapters {
		registry.adapters[adapter.Kind()] = adapter
		registry.order = append(registry.order, adapter)
	}
	return registry
}

func DirectRegistry() *Registry {
	return NewRegistry(15*time.Second, 5*time.Minute, DirectAdapter{})
}

func (r *Registry) Classify(rawURL string) (string, error) {
	for _, adapter := range r.order {
		if adapter.Match(rawURL) {
			return adapter.Kind(), nil
		}
	}
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.User == nil && len(rawURL) <= 2048 {
		return "", ErrUnsupported
	}
	return "", ErrInvalid
}

func (r *Registry) Resolve(ctx context.Context, kind, rawURL string) (Playable, error) {
	key := kind + "\x00" + rawURL
	r.mu.Lock()
	entry, found := r.cache[key]
	if found && time.Now().Before(entry.expiresAt) {
		r.mu.Unlock()
		return entry.value, nil
	}
	delete(r.cache, key)
	r.mu.Unlock()

	adapter, ok := r.adapters[kind]
	if !ok {
		return Playable{}, ErrUnsupported
	}
	resolveCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	value, err := adapter.Resolve(resolveCtx, rawURL)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return Playable{}, ErrUnavailable
		}
		return Playable{}, err
	}
	if r.cacheTTL > 0 {
		r.mu.Lock()
		r.cache[key] = cached{value: value, expiresAt: time.Now().Add(r.cacheTTL)}
		r.mu.Unlock()
	}
	return value, nil
}

type DirectAdapter struct{}

func (DirectAdapter) Kind() string { return "direct" }

func (DirectAdapter) Match(rawURL string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || len(rawURL) > 2048 {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host != "youtu.be" && host != "youtube.com" && !strings.HasSuffix(host, ".youtube.com")
}

func (adapter DirectAdapter) Resolve(_ context.Context, rawURL string) (Playable, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !adapter.Match(rawURL) {
		return Playable{}, ErrInvalid
	}
	return Playable{Kind: adapter.Kind(), OriginalURL: rawURL, PlaybackURL: rawURL}, nil
}
