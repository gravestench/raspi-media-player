package enrichment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ImageCache struct {
	dir    string
	client *http.Client
}

func NewImageCache(dir string, client *http.Client) *ImageCache {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &ImageCache{dir: dir, client: client}
}
func (c *ImageCache) Cache(ctx context.Context, key string, imageValue Image) (Image, error) {
	if c == nil || c.dir == "" || !validImage(imageValue) {
		return imageValue, nil
	}
	if err := os.MkdirAll(c.dir, 0750); err != nil {
		return imageValue, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageValue.URL, nil)
	if err != nil {
		return imageValue, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return imageValue, fmt.Errorf("download artist image: %w", providerSafeError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return imageValue, fmt.Errorf("artist image returned HTTP %d", resp.StatusCode)
	}
	mime := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
	if mime != "image/jpeg" && mime != "image/png" {
		return imageValue, errors.New("unsupported artist image type")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return imageValue, err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 120 || config.Height < 120 {
		return imageValue, errors.New("artist image is invalid or too small")
	}
	path := filepath.Join(c.dir, key+".image")
	temporary, err := os.CreateTemp(c.dir, key+"-*.tmp")
	if err != nil {
		return imageValue, err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return imageValue, err
	}
	if err := temporary.Chmod(0640); err != nil {
		temporary.Close()
		return imageValue, err
	}
	if err := temporary.Close(); err != nil {
		return imageValue, err
	}
	if err := os.Rename(name, path); err != nil {
		return imageValue, err
	}
	if err := os.WriteFile(filepath.Join(c.dir, key+".type"), []byte(mime), 0640); err != nil {
		return imageValue, err
	}
	imageValue.URL = "/api/v1/enrichment/images/" + key
	return imageValue, nil
}
func (c *ImageCache) Read(key string) ([]byte, string, error) {
	if c == nil || key == "" || strings.ContainsAny(key, "/\\.") {
		return nil, "", os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(c.dir, key+".image"))
	if err != nil {
		return nil, "", err
	}
	mime, err := os.ReadFile(filepath.Join(c.dir, key+".type"))
	if err != nil {
		return nil, "", err
	}
	return data, string(mime), nil
}
func (c *ImageCache) PruneOlderThan(age time.Duration) error {
	if c == nil || c.dir == "" {
		return nil
	}
	entries, err := os.ReadDir(c.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-age)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) && (strings.HasSuffix(entry.Name(), ".image") || strings.HasSuffix(entry.Name(), ".type") || strings.HasSuffix(entry.Name(), ".tmp")) {
			_ = os.Remove(filepath.Join(c.dir, entry.Name()))
		}
	}
	return nil
}
