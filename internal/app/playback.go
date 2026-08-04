package app

import (
	"errors"
	"net/http"

	"github.com/dylanknuth/raspi-media-player/internal/player"
)

type seekRequest struct {
	PositionSeconds float64 `json:"position_seconds"`
}
type volumeRequest struct {
	Volume int `json:"volume"`
}

func (a *application) pausePlayback(w http.ResponseWriter, r *http.Request) {
	a.playbackCommand(w, r, "pause", func() error { return a.playback.Pause(r.Context()) })
}
func (a *application) resumePlayback(w http.ResponseWriter, r *http.Request) {
	a.playbackCommand(w, r, "resume", func() error { return a.playback.Resume(r.Context()) })
}
func (a *application) stopPlayback(w http.ResponseWriter, r *http.Request) {
	a.playbackCommand(w, r, "stop", func() error { return a.playback.Stop(r.Context()) })
}

func (a *application) seekPlayback(w http.ResponseWriter, r *http.Request) {
	var request seekRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.PositionSeconds < 0 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_position", "position_seconds must be non-negative")
		return
	}
	a.playbackCommand(w, r, "seek", func() error { return a.playback.Seek(r.Context(), request.PositionSeconds) })
}

func (a *application) setPlaybackVolume(w http.ResponseWriter, r *http.Request) {
	var request volumeRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if request.Volume < 0 || request.Volume > 100 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_volume", "volume must be between 0 and 100")
		return
	}
	a.playbackCommand(w, r, "volume", func() error { return a.playback.SetVolume(r.Context(), request.Volume) })
}

func (a *application) playbackCommand(w http.ResponseWriter, r *http.Request, command string, operation func() error) {
	if a.playback == nil {
		writeError(w, http.StatusServiceUnavailable, "player_unavailable", "playback is not enabled")
		return
	}
	if err := operation(); err != nil {
		if errors.Is(err, player.ErrUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "player_unavailable", "player is unavailable")
			return
		}
		a.internalError(w, r, "playback "+command, err)
		return
	}
	loggerFromContext(r.Context(), a.logger).Info("playback command", "command", command)
	snapshot, err := a.queue.Snapshot(r.Context())
	if err != nil {
		a.internalError(w, r, "read playback state", err)
		return
	}
	writeSnapshot(w, http.StatusOK, snapshot)
}
