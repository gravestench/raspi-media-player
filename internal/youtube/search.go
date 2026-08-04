package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Channel   string  `json:"channel,omitempty"`
	Duration  float64 `json:"duration_seconds,omitempty"`
	Thumbnail string  `json:"thumbnail,omitempty"`
	URL       string  `json:"url"`
}

type Searcher interface {
	Search(context.Context, string, int) ([]Result, error)
}

type YTDLP struct {
	Binary  string
	Timeout time.Duration
}

func (s YTDLP) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 120 {
		return nil, errors.New("search query must be between 1 and 120 characters")
	}
	if limit < 1 || limit > 20 {
		limit = 8
	}
	binary := s.Binary
	if binary == "" {
		binary = "yt-dlp"
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, binary, "--dump-single-json", "--flat-playlist", "--no-warnings", "ytsearch"+strconv.Itoa(limit)+":"+query).Output()
	if commandCtx.Err() != nil {
		return nil, errors.New("YouTube search timed out")
	}
	if err != nil {
		return nil, fmt.Errorf("YouTube search failed: %w", err)
	}
	var response struct {
		Entries []struct {
			ID        string  `json:"id"`
			Title     string  `json:"title"`
			Uploader  string  `json:"uploader"`
			Channel   string  `json:"channel"`
			Duration  float64 `json:"duration"`
			Thumbnail string  `json:"thumbnail"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, errors.New("YouTube search returned invalid data")
	}
	results := make([]Result, 0, len(response.Entries))
	for _, entry := range response.Entries {
		if entry.ID == "" || entry.Title == "" {
			continue
		}
		channel := entry.Channel
		if channel == "" {
			channel = entry.Uploader
		}
		results = append(results, Result{ID: entry.ID, Title: entry.Title, Channel: channel, Duration: entry.Duration, Thumbnail: entry.Thumbnail, URL: "https://www.youtube.com/watch?v=" + entry.ID})
	}
	return results, nil
}
