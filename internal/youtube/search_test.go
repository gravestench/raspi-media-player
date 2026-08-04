package youtube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestYTDLPSearchMapsResultsWithoutShellExpansion(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(os.TempDir(), fmt.Sprintf("rmp-yt-pwn-%d", os.Getpid()))
	_ = os.Remove(marker)
	t.Cleanup(func() { _ = os.Remove(marker) })
	binary := filepath.Join(dir, "yt-dlp")
	script := "#!/bin/sh\nprintf '%s' '{\"entries\":[{\"id\":\"abc123\",\"title\":\"A Song\",\"channel\":\"The Artist\",\"duration\":125,\"thumbnail\":\"https://i.ytimg.com/a.jpg\"}]}'\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	results, err := (YTDLP{Binary: binary, Timeout: time.Second}).Search(context.Background(), "song; touch "+marker, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].URL != "https://www.youtube.com/watch?v=abc123" || results[0].Channel != "The Artist" {
		t.Fatalf("results=%+v", results)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("search query was interpreted by a shell")
	}
}

func TestYTDLPSearchValidatesQuery(t *testing.T) {
	if _, err := (YTDLP{}).Search(context.Background(), "", 8); err == nil {
		t.Fatal("empty query accepted")
	}
}
