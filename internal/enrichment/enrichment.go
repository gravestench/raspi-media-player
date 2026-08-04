package enrichment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
)

type TrackHint struct {
	Artist   string `json:"artist"`
	Title    string `json:"title"`
	RawTitle string `json:"raw_title"`
}
type Image struct {
	URL         string `json:"url"`
	SourceURL   string `json:"source_url"`
	Attribution string `json:"attribution"`
}
type ArtistSummary struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}
type Result struct {
	CacheKey       string          `json:"cache_key"`
	Hint           TrackHint       `json:"hint"`
	Provider       string          `json:"provider,omitempty"`
	ArtistURL      string          `json:"artist_url,omitempty"`
	Image          Image           `json:"image"`
	Biography      string          `json:"biography,omitempty"`
	Genres         []string        `json:"genres"`
	RelatedArtists []ArtistSummary `json:"related_artists"`
	Status         string          `json:"status"`
	ErrorCode      string          `json:"error_code,omitempty"`
	FetchedAt      string          `json:"fetched_at,omitempty"`
	ExpiresAt      string          `json:"expires_at"`
}
type Provider interface {
	Name() string
	Lookup(context.Context, TrackHint) (Result, error)
}

var suffixPattern = regexp.MustCompile(`(?i)\s*[\[(](official\s+)?(music\s+)?(video|audio|visuali[sz]er|lyric(s)?(\s+video)?|live)[^\])]*[\])]\s*$`)

func ParseTitle(raw string) TrackHint {
	cleaned := strings.TrimSpace(strings.Join(strings.Fields(raw), " "))
	for {
		next := strings.TrimSpace(suffixPattern.ReplaceAllString(cleaned, ""))
		if next == cleaned {
			break
		}
		cleaned = next
	}
	for _, separator := range []string{" - ", " – ", " — "} {
		if before, after, found := strings.Cut(cleaned, separator); found {
			artist, title := strings.TrimSpace(before), strings.TrimSpace(after)
			if artist != "" && title != "" {
				return TrackHint{Artist: artist, Title: title, RawTitle: strings.TrimSpace(raw)}
			}
		}
	}
	return TrackHint{Title: cleaned, RawTitle: strings.TrimSpace(raw)}
}

func Key(hint TrackHint) string {
	sum := sha256.Sum256([]byte(normalize(hint.Artist) + "\x00" + normalize(hint.Title)))
	return hex.EncodeToString(sum[:16])
}
func normalize(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		if unicode.IsSpace(r) {
			return ' '
		}
		return -1
	}, strings.TrimSpace(value))
}
