package player

import (
	"context"
	"sync"
)

type Fake struct {
	mu         sync.Mutex
	events     chan Event
	LoadedURLs []string
	State      State
}

func NewFake() *Fake {
	return &Fake{events: make(chan Event, 32), State: State{Status: "idle", Volume: 100}}
}
func (f *Fake) Start(context.Context) error { return nil }
func (f *Fake) Load(_ context.Context, value string) error {
	f.mu.Lock()
	f.LoadedURLs = append(f.LoadedURLs, value)
	f.State.Status = "playing"
	state := f.State
	f.mu.Unlock()
	f.events <- Event{Type: EventState, State: state}
	return nil
}
func (f *Fake) SetPaused(_ context.Context, value bool) error {
	f.mu.Lock()
	f.State.Paused = value
	if value {
		f.State.Status = "paused"
	} else {
		f.State.Status = "playing"
	}
	state := f.State
	f.mu.Unlock()
	f.events <- Event{Type: EventState, State: state}
	return nil
}
func (f *Fake) Stop(context.Context) error {
	f.mu.Lock()
	f.State.Status = "stopped"
	state := f.State
	f.mu.Unlock()
	f.events <- Event{Type: EventState, State: state}
	return nil
}
func (f *Fake) Seek(_ context.Context, value float64) error {
	f.mu.Lock()
	f.State.PositionSeconds = value
	state := f.State
	f.mu.Unlock()
	f.events <- Event{Type: EventState, State: state}
	return nil
}
func (f *Fake) SetVolume(_ context.Context, value int) error {
	f.mu.Lock()
	f.State.Volume = value
	state := f.State
	f.mu.Unlock()
	f.events <- Event{Type: EventState, State: state}
	return nil
}
func (f *Fake) Events() <-chan Event { return f.events }
func (f *Fake) Close() error         { return nil }
func (f *Fake) Emit(event Event)     { f.events <- event }
func (f *Fake) URLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.LoadedURLs...)
}
func (f *Fake) Snapshot() State { f.mu.Lock(); defer f.mu.Unlock(); return f.State }
