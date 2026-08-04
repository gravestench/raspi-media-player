package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type TagArtist struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type TagTrack struct {
	Name      string `json:"name"`
	URL       string `json:"url,omitempty"`
	Artist    string `json:"artist"`
	ArtistURL string `json:"artist_url,omitempty"`
}

type TagDiscovery struct {
	Genre   string      `json:"genre"`
	Artists []TagArtist `json:"artists"`
	Tracks  []TagTrack  `json:"tracks"`
}

func (p *LastFMProvider) DiscoverTag(ctx context.Context, tag string, limit int) (TagDiscovery, error) {
	tag = strings.TrimSpace(tag)
	if !p.Enabled() {
		return TagDiscovery{}, errors.New("last.fm provider is not configured")
	}
	if tag == "" || len(tag) > 120 {
		return TagDiscovery{}, errors.New("genre must be between 1 and 120 characters")
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	type response struct {
		method string
		data   []byte
		err    error
	}
	responses := make(chan response, 2)
	for _, method := range []string{"tag.getTopArtists", "tag.getTopTracks"} {
		go func(method string) {
			data, err := p.tagRequest(ctx, method, tag, limit)
			responses <- response{method: method, data: data, err: err}
		}(method)
	}
	result := TagDiscovery{Genre: tag, Artists: []TagArtist{}, Tracks: []TagTrack{}}
	for range 2 {
		value := <-responses
		if value.err != nil {
			return TagDiscovery{}, value.err
		}
		switch value.method {
		case "tag.getTopArtists":
			var payload struct {
				TopArtists struct {
					Artists []TagArtist `json:"artist"`
				} `json:"topartists"`
				Error int `json:"error"`
			}
			if err := json.Unmarshal(value.data, &payload); err != nil || payload.Error != 0 {
				return TagDiscovery{}, errors.New("decode last.fm artist discovery response")
			}
			result.Artists = payload.TopArtists.Artists
		case "tag.getTopTracks":
			var payload struct {
				Tracks struct {
					Tracks []struct {
						Name   string `json:"name"`
						URL    string `json:"url"`
						Artist struct {
							Name string `json:"name"`
							URL  string `json:"url"`
						} `json:"artist"`
					} `json:"track"`
				} `json:"tracks"`
				Error int `json:"error"`
			}
			if err := json.Unmarshal(value.data, &payload); err != nil || payload.Error != 0 {
				return TagDiscovery{}, errors.New("decode last.fm track discovery response")
			}
			for _, track := range payload.Tracks.Tracks {
				if strings.TrimSpace(track.Name) != "" && strings.TrimSpace(track.Artist.Name) != "" {
					result.Tracks = append(result.Tracks, TagTrack{Name: track.Name, URL: track.URL, Artist: track.Artist.Name, ArtistURL: track.Artist.URL})
				}
			}
		}
	}
	return result, nil
}

func (p *LastFMProvider) tagRequest(ctx context.Context, method, tag string, limit int) ([]byte, error) {
	endpoint, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, errors.New("invalid last.fm provider endpoint")
	}
	query := endpoint.Query()
	query.Set("method", method)
	query.Set("tag", tag)
	query.Set("limit", strconv.Itoa(limit))
	query.Set("api_key", p.apiKey)
	query.Set("format", "json")
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("create last.fm discovery request")
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("last.fm discovery request failed: %w", providerSafeError(err))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("last.fm returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, errors.New("read last.fm discovery response")
	}
	return data, nil
}
