package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type MusicBrainzProvider struct {
	baseURL     string
	client      *http.Client
	userAgent   string
	interval    time.Duration
	mu          sync.Mutex
	nextRequest time.Time
}

func NewMusicBrainzProvider(userAgent string, client *http.Client) *MusicBrainzProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &MusicBrainzProvider{baseURL: "https://musicbrainz.org/ws/2/", client: client, userAgent: strings.TrimSpace(userAgent), interval: time.Second}
}
func (p *MusicBrainzProvider) Name() string { return "musicbrainz" }
func (p *MusicBrainzProvider) Lookup(ctx context.Context, hint TrackHint) (Result, error) {
	if hint.Artist == "" {
		return Result{}, ErrProviderNotFound
	}
	if p.userAgent == "" {
		return Result{}, errors.New("musicbrainz user agent is required")
	}
	searchURL := p.baseURL + "artist/?fmt=json&limit=1&query=" + url.QueryEscape(`artist:"`+hint.Artist+`"`)
	var search struct {
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	}
	if err := p.get(ctx, searchURL, &search); err != nil {
		return Result{}, err
	}
	if len(search.Artists) == 0 {
		return Result{}, ErrProviderNotFound
	}
	lookupURL := p.baseURL + "artist/" + url.PathEscape(search.Artists[0].ID) + "?fmt=json&inc=artist-rels+url-rels+tags"
	var artist struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Disambiguation string `json:"disambiguation"`
		Tags           []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"tags"`
		Relations []struct {
			Type   string `json:"type"`
			Artist *struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"artist"`
			URL *struct {
				Resource string `json:"resource"`
			} `json:"url"`
		} `json:"relations"`
	}
	if err := p.get(ctx, lookupURL, &artist); err != nil {
		return Result{}, err
	}
	result := Result{Hint: hint, Provider: p.Name(), ArtistURL: "https://musicbrainz.org/artist/" + artist.ID, Genres: []string{}, RelatedArtists: []ArtistSummary{}, Status: "ready"}
	result.Hint.Artist = artist.Name
	for _, tag := range artist.Tags {
		if tag.Name != "" && len(result.Genres) < 8 {
			result.Genres = append(result.Genres, tag.Name)
		}
	}
	for _, relation := range artist.Relations {
		if relation.Artist != nil && relation.Artist.Name != "" && len(result.RelatedArtists) < 8 {
			result.RelatedArtists = append(result.RelatedArtists, ArtistSummary{Name: relation.Artist.Name, URL: "https://musicbrainz.org/artist/" + relation.Artist.ID})
		}
	}
	return result, nil
}
func (p *MusicBrainzProvider) get(ctx context.Context, endpoint string, destination any) error {
	p.mu.Lock()
	wait := time.Until(p.nextRequest)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			p.mu.Unlock()
			return ctx.Err()
		case <-timer.C:
		}
	}
	p.nextRequest = time.Now().Add(p.interval)
	p.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("create musicbrainz request")
	}
	req.Header.Set("User-Agent", p.userAgent)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("musicbrainz request failed: %w", providerSafeError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("musicbrainz returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(destination); err != nil {
		return errors.New("decode musicbrainz response")
	}
	return nil
}
