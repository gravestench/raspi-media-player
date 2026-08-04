package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type MPVConfig struct {
	Binary       string
	SocketPath   string
	AudioDevice  string
	CacheSeconds int
}
type MPV struct {
	logger    *slog.Logger
	config    MPVConfig
	events    chan Event
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	conn      net.Conn
	pending   map[int64]chan mpvMessage
	requestID atomic.Int64
	state     State
	readyOnce sync.Once
	ready     chan error
}
type mpvMessage struct {
	Event     string `json:"event"`
	Name      string `json:"name"`
	Data      any    `json:"data"`
	Reason    string `json:"reason"`
	Error     string `json:"error"`
	RequestID int64  `json:"request_id"`
}

func NewMPV(logger *slog.Logger, config MPVConfig) *MPV {
	if config.Binary == "" {
		config.Binary = "mpv"
	}
	if config.SocketPath == "" {
		config.SocketPath = "/var/lib/raspi-media-player/mpv.sock"
	}
	if config.CacheSeconds <= 0 {
		config.CacheSeconds = 20
	}
	return &MPV{logger: logger, config: config, events: make(chan Event, 128), pending: make(map[int64]chan mpvMessage), state: State{Status: "idle", Volume: 100}, ready: make(chan error, 1)}
}

func (m *MPV) Start(parent context.Context) error {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return nil
	}
	m.ctx, m.cancel = context.WithCancel(parent)
	m.mu.Unlock()
	go m.supervise()
	select {
	case err := <-m.ready:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("timed out starting mpv")
	case <-parent.Done():
		return parent.Err()
	}
}

func (m *MPV) supervise() {
	delay := time.Second
	for m.ctx.Err() == nil {
		err := m.runOnce()
		if m.ctx.Err() != nil {
			return
		}
		m.logger.Error("mpv process stopped", "error", err, "restart_delay", delay)
		m.emit(Event{Type: EventState, State: State{Status: "unavailable", Error: err.Error()}})
		select {
		case <-time.After(delay):
		case <-m.ctx.Done():
			return
		}
		if delay < 10*time.Second {
			delay *= 2
		}
	}
}

func (m *MPV) runOnce() error {
	if err := os.MkdirAll(filepath.Dir(m.config.SocketPath), 0750); err != nil {
		m.signalReady(err)
		return err
	}
	_ = os.Remove(m.config.SocketPath)
	args := []string{"--idle=yes", "--no-video", "--no-terminal", "--input-ipc-server=" + m.config.SocketPath, "--cache=yes", "--cache-secs=" + strconv.Itoa(m.config.CacheSeconds), "--network-timeout=30", "--audio-buffer=2.0"}
	if m.config.AudioDevice != "" && m.config.AudioDevice != "auto" {
		args = append(args, "--ao=alsa", "--audio-device="+m.config.AudioDevice)
	}
	cmd := exec.CommandContext(m.ctx, m.config.Binary, args...)
	if err := cmd.Start(); err != nil {
		m.signalReady(err)
		return fmt.Errorf("start mpv: %w", err)
	}
	m.logger.Info("mpv process started", "pid", cmd.Process.Pid, "socket", m.config.SocketPath, "audio_device", m.config.AudioDevice)
	waitResult := make(chan error, 1)
	go func() { waitResult <- cmd.Wait() }()
	var conn net.Conn
	var err error
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = net.DialTimeout("unix", m.config.SocketPath, 250*time.Millisecond)
		if err == nil {
			break
		}
		select {
		case processErr := <-waitResult:
			if processErr == nil {
				processErr = errors.New("mpv exited")
			}
			m.signalReady(processErr)
			return fmt.Errorf("mpv exited before IPC became ready: %w", processErr)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	if conn == nil {
		_ = cmd.Process.Kill()
		m.signalReady(err)
		return fmt.Errorf("connect to mpv IPC: %w", err)
	}
	m.mu.Lock()
	m.conn = conn
	m.mu.Unlock()
	readerDone := make(chan error, 1)
	go func() { readerDone <- m.readLoop(conn) }()
	m.signalReady(nil)
	for index, property := range []string{"media-title", "time-pos", "duration", "pause", "paused-for-cache", "volume", "core-idle"} {
		_ = m.send(m.ctx, []any{"observe_property", index + 1, property})
	}
	select {
	case processErr := <-waitResult:
		_ = conn.Close()
		m.clearConnection(conn)
		if processErr == nil {
			processErr = errors.New("mpv exited")
		}
		return processErr
	case readErr := <-readerDone:
		_ = cmd.Process.Kill()
		processErr := <-waitResult
		m.clearConnection(conn)
		if readErr != nil {
			return readErr
		}
		if processErr == nil {
			processErr = errors.New("mpv exited")
		}
		return processErr
	case <-m.ctx.Done():
		_ = conn.Close()
		_ = cmd.Process.Kill()
		<-waitResult
		m.clearConnection(conn)
		return m.ctx.Err()
	}
}

func (m *MPV) signalReady(err error) { m.readyOnce.Do(func() { m.ready <- err }) }
func (m *MPV) clearConnection(conn net.Conn) {
	m.mu.Lock()
	if m.conn == conn {
		m.conn = nil
	}
	for id, channel := range m.pending {
		close(channel)
		delete(m.pending, id)
	}
	m.mu.Unlock()
}

func (m *MPV) readLoop(conn net.Conn) error {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		var message mpvMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			m.logger.Warn("invalid mpv IPC message", "error", err)
			continue
		}
		if message.RequestID != 0 {
			m.mu.Lock()
			response := m.pending[message.RequestID]
			delete(m.pending, message.RequestID)
			m.mu.Unlock()
			if response != nil {
				response <- message
				close(response)
			}
			continue
		}
		m.handleEvent(message)
	}
	return scanner.Err()
}

func (m *MPV) handleEvent(message mpvMessage) {
	if message.Event == "end-file" {
		if message.Reason == "error" {
			m.emit(Event{Type: EventFailed, Error: errors.New(message.Error), State: m.snapshot()})
		} else if message.Reason == "eof" {
			m.emit(Event{Type: EventEnded, State: m.snapshot()})
		}
		return
	}
	if message.Event != "property-change" {
		return
	}
	m.mu.Lock()
	switch message.Name {
	case "media-title":
		if value, ok := message.Data.(string); ok {
			m.state.Title = value
		}
	case "time-pos":
		if value, ok := message.Data.(float64); ok {
			m.state.PositionSeconds = value
		}
	case "duration":
		if value, ok := message.Data.(float64); ok {
			m.state.DurationSeconds = value
		}
	case "pause":
		if value, ok := message.Data.(bool); ok {
			m.state.Paused = value
		}
	case "paused-for-cache":
		if value, ok := message.Data.(bool); ok {
			m.state.Buffering = value
		}
	case "volume":
		if value, ok := message.Data.(float64); ok {
			m.state.Volume = int(value + .5)
		}
	case "core-idle":
		if value, ok := message.Data.(bool); ok && value {
			m.state.Status = "idle"
		}
	}
	if m.state.Buffering {
		m.state.Status = "buffering"
	} else if m.state.Paused {
		m.state.Status = "paused"
	} else if m.state.Status != "idle" {
		m.state.Status = "playing"
	}
	state := m.state
	m.mu.Unlock()
	m.emit(Event{Type: EventState, State: state})
}

func (m *MPV) send(ctx context.Context, command []any) error {
	id := m.requestID.Add(1)
	response := make(chan mpvMessage, 1)
	m.mu.Lock()
	conn := m.conn
	if conn == nil {
		m.mu.Unlock()
		return ErrUnavailable
	}
	m.pending[id] = response
	payload, err := json.Marshal(map[string]any{"command": command, "request_id": id})
	if err == nil {
		payload = append(payload, '\n')
		_, err = conn.Write(payload)
	}
	if err != nil {
		delete(m.pending, id)
	}
	m.mu.Unlock()
	if err != nil {
		return err
	}
	select {
	case value, ok := <-response:
		if !ok {
			return ErrUnavailable
		}
		if value.Error != "" && value.Error != "success" {
			return errors.New(value.Error)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return errors.New("mpv command timed out")
	}
}

func (m *MPV) Load(ctx context.Context, value string) error {
	m.mu.Lock()
	m.state = State{Status: "loading", Volume: m.state.Volume}
	state := m.state
	m.mu.Unlock()
	m.emit(Event{Type: EventState, State: state})
	return m.send(ctx, []any{"loadfile", value, "replace"})
}
func (m *MPV) SetPaused(ctx context.Context, value bool) error {
	return m.send(ctx, []any{"set_property", "pause", value})
}
func (m *MPV) Stop(ctx context.Context) error { return m.send(ctx, []any{"stop"}) }
func (m *MPV) Seek(ctx context.Context, value float64) error {
	return m.send(ctx, []any{"seek", value, "absolute", "exact"})
}
func (m *MPV) SetVolume(ctx context.Context, value int) error {
	return m.send(ctx, []any{"set_property", "volume", value})
}
func (m *MPV) Events() <-chan Event { return m.events }
func (m *MPV) emit(event Event) {
	select {
	case m.events <- event:
	default:
		m.logger.Warn("dropping player event", "event_type", event.Type)
	}
}
func (m *MPV) snapshot() State { m.mu.Lock(); defer m.mu.Unlock(); return m.state }
func (m *MPV) Close() error {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}
