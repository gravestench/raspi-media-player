package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type WikimediaProvider struct {
	wikidataURL, commonsURL string
	client                  *http.Client
	userAgent               string
}

func NewWikimediaProvider(userAgent string, client *http.Client) *WikimediaProvider {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &WikimediaProvider{wikidataURL: "https://www.wikidata.org/w/api.php", commonsURL: "https://commons.wikimedia.org/w/api.php", client: client, userAgent: strings.TrimSpace(userAgent)}
}
func (p *WikimediaProvider) Name() string { return "wikimedia" }
func (p *WikimediaProvider) Lookup(ctx context.Context, hint TrackHint) (Result, error) {
	if hint.Artist == "" {
		return Result{}, ErrProviderNotFound
	}
	var search struct {
		Search []struct{ ID, Label, Description string } `json:"search"`
	}
	if err := p.get(ctx, p.wikidataURL, map[string]string{"action": "wbsearchentities", "search": hint.Artist, "language": "en", "type": "item", "limit": "5", "format": "json"}, &search); err != nil {
		return Result{}, err
	}
	if len(search.Search) == 0 {
		return Result{}, ErrProviderNotFound
	}
	id := ""
	for _, candidate := range search.Search {
		description := strings.ToLower(candidate.Description)
		if strings.Contains(description, "singer") || strings.Contains(description, "musician") || strings.Contains(description, "band") || strings.Contains(description, "rapper") || strings.Contains(description, "composer") || strings.Contains(description, "songwriter") || strings.Contains(description, "music group") {
			id = candidate.ID
			break
		}
	}
	if id == "" {
		return Result{}, ErrProviderNotFound
	}
	var entity struct {
		Entities map[string]struct {
			Claims map[string][]struct {
				Mainsnak struct {
					Datavalue struct {
						Value string `json:"value"`
					} `json:"datavalue"`
				} `json:"mainsnak"`
			} `json:"claims"`
		} `json:"entities"`
	}
	if err := p.get(ctx, p.wikidataURL, map[string]string{"action": "wbgetentities", "ids": id, "props": "claims", "format": "json"}, &entity); err != nil {
		return Result{}, err
	}
	claims := entity.Entities[id].Claims["P18"]
	if len(claims) == 0 {
		return Result{}, ErrProviderNotFound
	}
	filename := claims[0].Mainsnak.Datavalue.Value
	if filename == "" {
		return Result{}, ErrProviderNotFound
	}
	title := "File:" + filename
	var images struct {
		Query struct {
			Pages map[string]struct {
				ImageInfo []struct {
					ThumbURL       string `json:"thumburl"`
					DescriptionURL string `json:"descriptionurl"`
					ExtMetadata    map[string]struct {
						Value string `json:"value"`
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := p.get(ctx, p.commonsURL, map[string]string{"action": "query", "titles": title, "prop": "imageinfo", "iiprop": "url|extmetadata", "iiurlwidth": "600", "format": "json"}, &images); err != nil {
		return Result{}, err
	}
	for _, page := range images.Query.Pages {
		if len(page.ImageInfo) == 0 {
			continue
		}
		info := page.ImageInfo[0]
		license := plainMetadata(info.ExtMetadata["LicenseShortName"].Value)
		artist := plainMetadata(info.ExtMetadata["Artist"].Value)
		credit := plainMetadata(info.ExtMetadata["Credit"].Value)
		attribution := strings.TrimSpace(strings.Join(nonEmpty([]string{artist, credit, license}), " · "))
		if info.ThumbURL == "" || info.DescriptionURL == "" || attribution == "" {
			return Result{}, ErrProviderNotFound
		}
		return Result{Hint: hint, Provider: p.Name(), ArtistURL: "https://www.wikidata.org/wiki/" + id, Image: Image{URL: info.ThumbURL, SourceURL: info.DescriptionURL, Attribution: attribution}, Genres: []string{}, RelatedArtists: []ArtistSummary{}, Status: "ready"}, nil
	}
	return Result{}, ErrProviderNotFound
}
func (p *WikimediaProvider) get(ctx context.Context, base string, params map[string]string, destination any) error {
	endpoint, err := url.Parse(base)
	if err != nil {
		return errors.New("invalid wikimedia endpoint")
	}
	query := endpoint.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return errors.New("create wikimedia request")
	}
	if p.userAgent != "" {
		req.Header.Set("User-Agent", p.userAgent)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("wikimedia request failed: %w", providerSafeError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wikimedia returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(destination); err != nil {
		return errors.New("decode wikimedia response")
	}
	return nil
}
func plainMetadata(value string) string {
	return strings.TrimSpace(html.UnescapeString(htmlPattern.ReplaceAllString(value, "")))
}
func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}
