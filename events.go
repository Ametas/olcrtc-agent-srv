// events.go turns the tunnel's lifecycle/traffic/health hooks into single-line
// structured JSON written to an io.Writer (stdout in production). systemd
// captures stdout into journald, and route-agent greps/tails those lines for
// live telemetry — the same "exec+parse, never a stored flag" philosophy the
// rest of the fleet's telemetry already follows (singbox_version-style).
//
// One JSON object per line, always with a stable "type" field first so a line
// -oriented consumer can dispatch without buffering multi-line JSON.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/openlibrecommunity/olcrtc/pkg/olcrtc/tunnel"
)

const (
	EventTypeSessionOpen  = "session_open"
	EventTypeSessionClose = "session_close"
	EventTypeTraffic      = "traffic"
	EventTypeHealth       = "health"
)

// Event is the single wire shape for every emitted line. Fields not relevant
// to a given Type are simply omitted (omitempty) rather than modeled as a
// union of Go types — keeps the consumer side (a line-oriented grep/parse in
// route-agent) trivial.
type Event struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"ts"`
	SessionID string          `json:"session_id,omitempty"`
	DeviceID  string          `json:"device_id,omitempty"`
	Reason    string          `json:"reason,omitempty"`
	Addr      string          `json:"addr,omitempty"`
	BytesIn   uint64          `json:"bytes_in,omitempty"`
	BytesOut  uint64          `json:"bytes_out,omitempty"`
	Claims    map[string]any  `json:"claims,omitempty"`
	Health    json.RawMessage `json:"health,omitempty"`
}

// EventEmitter serializes concurrent hook calls into atomic single-line
// writes. The tunnel invokes OnSessionOpen/OnSessionClose/OnTraffic/OnHealth
// from its own internal goroutines per session, so writes to a shared
// io.Writer (os.Stdout) need a lock to avoid interleaved partial lines.
type EventEmitter struct {
	mu  sync.Mutex
	out io.Writer
	// now is overridable for tests; defaults to time.Now.
	now func() time.Time
}

// NewEventEmitter wraps w (typically os.Stdout).
func NewEventEmitter(w io.Writer) *EventEmitter {
	return &EventEmitter{out: w, now: time.Now}
}

func (e *EventEmitter) emit(ev Event) {
	ev.Timestamp = e.now()
	line, err := json.Marshal(ev)
	if err != nil {
		// A logging failure must never take down the tunnel itself — fall
		// back to a best-effort plain-text line so the failure is at least
		// visible instead of silently dropped.
		e.mu.Lock()
		fmt.Fprintf(e.out, "{\"type\":\"emit_error\",\"error\":%q}\n", err.Error())
		e.mu.Unlock()
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.out.Write(line)
	e.out.Write([]byte("\n"))
}

// OnSessionOpen matches tunnel.SessionOpenFunc.
func (e *EventEmitter) OnSessionOpen(sessionID, deviceID string, claims map[string]any) {
	e.emit(Event{Type: EventTypeSessionOpen, SessionID: sessionID, DeviceID: deviceID, Claims: claims})
}

// OnSessionClose matches tunnel.SessionCloseFunc.
func (e *EventEmitter) OnSessionClose(sessionID, reason string) {
	e.emit(Event{Type: EventTypeSessionClose, SessionID: sessionID, Reason: reason})
}

// OnTraffic matches tunnel.TrafficFunc. bytesIn/bytesOut are for one tunnel
// stream, reported after both copy loops finish (not a running counter) —
// see tunnel.go's TrafficFunc doc comment upstream.
func (e *EventEmitter) OnTraffic(sessionID, addr string, bytesIn, bytesOut uint64) {
	e.emit(Event{Type: EventTypeTraffic, SessionID: sessionID, Addr: addr, BytesIn: bytesIn, BytesOut: bytesOut})
}

// OnHealth matches tunnel.HealthFunc. The upstream HealthStatus type
// (control.Status) isn't re-declared here — we round-trip it through
// json.Marshal generically so this file doesn't need to track its exact
// field set across upstream versions; only a change to a non-JSON-safe shape
// would break this, which go vet/the emit_error fallback above would surface.
func (e *EventEmitter) OnHealth(status tunnel.HealthStatus) {
	health, err := json.Marshal(status)
	if err != nil {
		e.emit(Event{Type: EventTypeHealth, Reason: fmt.Sprintf("marshal health: %v", err)})
		return
	}
	e.emit(Event{Type: EventTypeHealth, Health: health})
}
