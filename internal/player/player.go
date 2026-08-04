package player

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("player unavailable")

type State struct {
	Status          string  `json:"status"`
	Title           string  `json:"title,omitempty"`
	PositionSeconds float64 `json:"position_seconds"`
	DurationSeconds float64 `json:"duration_seconds"`
	Paused          bool    `json:"paused"`
	Buffering       bool    `json:"buffering"`
	Volume          int     `json:"volume"`
	Error           string  `json:"error,omitempty"`
}

type EventType string

const (
	EventState  EventType = "state"
	EventEnded  EventType = "ended"
	EventFailed EventType = "failed"
)

type Event struct {
	Type  EventType
	State State
	Error error
}

type Player interface {
	Start(context.Context) error
	Load(context.Context, string) error
	SetPaused(context.Context, bool) error
	Stop(context.Context) error
	Seek(context.Context, float64) error
	SetVolume(context.Context, int) error
	Events() <-chan Event
	Close() error
}
