package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (a *application) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unavailable", "event streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	controller := http.NewResponseController(w)
	ticker := time.NewTicker(500 * time.Millisecond)
	keepalive := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	defer keepalive.Stop()
	var previous []byte
	send := func() error {
		snapshot, err := a.queue.Snapshot(r.Context())
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		if bytes.Equal(encoded, previous) {
			return nil
		}
		previous = append(previous[:0], encoded...)
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := fmt.Fprintf(w, "event: queue\ndata: %s\n\n", encoded); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	if err := send(); err != nil {
		loggerFromContext(r.Context(), a.logger).Warn("event stream initial snapshot failed", "error", err)
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := send(); err != nil {
				return
			}
		case <-keepalive.C:
			_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
