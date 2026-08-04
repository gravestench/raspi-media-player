package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var ErrProviderNotFound = errors.New("artist metadata not found")

type LastFMProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewLastFMProvider(apiKey string, client *http.Client) *LastFMProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &LastFMProvider{apiKey: strings.TrimSpace(apiKey), baseURL: "https://ws.audioscrobbler.com/2.0/", client: client}
}

func (p *LastFMProvider) Name() string  { return "lastfm" }
func (p *LastFMProvider) Enabled() bool { return p.apiKey != "" }

func (p *LastFMProvider) Lookup(ctx context.Context, hint TrackHint) (Result, error) {
	if !p.Enabled() {
		return Result{}, errors.New("last.fm provider is not configured")
	}
	if strings.TrimSpace(hint.Artist) == "" {
		return Result{}, ErrProviderNotFound
	}
	endpoint, err := url.Parse(p.baseURL)
	if err != nil {
		return Result{}, errors.New("invalid last.fm provider endpoint")
	}
	query := endpoint.Query()
	query.Set("method", "artist.getInfo")
	query.Set("artist", hint.Artist)
	query.Set("autocorrect", "1")
	query.Set("api_key", p.apiKey)
	query.Set("format", "json")
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Result{}, errors.New("create last.fm request")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("last.fm request failed: %w", providerSafeError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("last.fm returned HTTP %d", resp.StatusCode)
	}
	var payload lastFMResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		return Result{}, errors.New("decode last.fm response")
	}
	if payload.Error != 0 || payload.Artist.Name == "" {
		return Result{}, ErrProviderNotFound
	}
	result := Result{Hint: hint, Provider: p.Name(), ArtistURL: payload.Artist.URL, Biography: cleanBiography(payload.Artist.Bio.Summary), Genres: make([]string, 0), RelatedArtists: make([]ArtistSummary, 0), Status: "ready"}
	result.Hint.Artist = payload.Artist.Name
	for _, tag := range payload.Artist.Tags.Tag {
		if name := strings.TrimSpace(tag.Name); name != "" && len(result.Genres) < 8 {
			result.Genres = append(result.Genres, name)
		}
	}
	for _, related := range payload.Artist.Similar.Artist {
		if name := strings.TrimSpace(related.Name); name != "" && len(result.RelatedArtists) < 8 {
			result.RelatedArtists = append(result.RelatedArtists, ArtistSummary{Name: name, URL: related.URL})
		}
	}
	for index := len(payload.Artist.Image) - 1; index >= 0; index-- {
		if imageURL := strings.TrimSpace(payload.Artist.Image[index].URL); imageURL != "" {
			result.Image = Image{URL: imageURL, SourceURL: payload.Artist.URL, Attribution: "Artist image via Last.fm"}
			break
		}
	}
	return result, nil
}

type lastFMResponse struct {
	Error   int    `json:"error"`
	Message string `json:"message"`
	Artist  struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Image []struct {
			URL  string `json:"#text"`
			Size string `json:"size"`
		} `json:"image"`
		Tags struct {
			Tag []struct {
				Name string `json:"name"`
			} `json:"tag"`
		} `json:"tags"`
		Similar struct {
			Artist []struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"artist"`
		} `json:"similar"`
		Bio struct {
			Summary string `json:"summary"`
		} `json:"bio"`
	} `json:"artist"`
}

var htmlPattern = regexp.MustCompile(`<[^>]+>`)

func cleanBiography(value string) string {
	return strings.TrimSpace(htmlPattern.ReplaceAllString(value, ""))
}
func providerSafeError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	return errors.New("provider unavailable")
}
