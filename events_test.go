package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openlibrecommunity/olcrtc/pkg/olcrtc/tunnel"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestEventEmitter_SessionOpen(t *testing.T) {
	var buf bytes.Buffer
	e := NewEventEmitter(&buf)
	e.now = fixedClock(time.Unix(1000, 0).UTC())

	e.OnSessionOpen("sess-1", "device-42", map[string]any{"foo": "bar"})

	var got Event
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal emitted line: %v (raw: %s)", err, buf.String())
	}
	if got.Type != EventTypeSessionOpen {
		t.Errorf("Type = %q, want %q", got.Type, EventTypeSessionOpen)
	}
	if got.SessionID != "sess-1" || got.DeviceID != "device-42" {
		t.Errorf("SessionID/DeviceID = %q/%q", got.SessionID, got.DeviceID)
	}
	if got.Claims["foo"] != "bar" {
		t.Errorf("Claims = %+v", got.Claims)
	}
	if !got.Timestamp.Equal(time.Unix(1000, 0).UTC()) {
		t.Errorf("Timestamp = %v", got.Timestamp)
	}
}

func TestEventEmitter_SessionClose(t *testing.T) {
	var buf bytes.Buffer
	e := NewEventEmitter(&buf)
	e.OnSessionClose("sess-1", "peer disconnected")

	var got Event
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != EventTypeSessionClose || got.Reason != "peer disconnected" {
		t.Errorf("got = %+v", got)
	}
}

func TestEventEmitter_Traffic(t *testing.T) {
	var buf bytes.Buffer
	e := NewEventEmitter(&buf)
	e.OnTraffic("sess-1", "10.0.0.5:443", 1024, 2048)

	var got Event
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.BytesIn != 1024 || got.BytesOut != 2048 || got.Addr != "10.0.0.5:443" {
		t.Errorf("got = %+v", got)
	}
}

func TestEventEmitter_Health(t *testing.T) {
	var buf bytes.Buffer
	e := NewEventEmitter(&buf)

	var status tunnel.HealthStatus
	e.OnHealth(status)

	var got Event
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != EventTypeHealth {
		t.Errorf("Type = %q, want %q", got.Type, EventTypeHealth)
	}
}

// TestEventEmitter_ConcurrentWritesStayAtomic guards the whole reason emit()
// takes a mutex: the tunnel calls these hooks from per-session goroutines, so
// interleaved writes to the shared stdout stream would corrupt line-oriented
// JSON parsing on the route-agent side.
func TestEventEmitter_ConcurrentWritesStayAtomic(t *testing.T) {
	var buf bytes.Buffer
	e := NewEventEmitter(&buf)

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			e.OnTraffic("sess", "addr", uint64(i), uint64(i))
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("got %d lines, want %d (interleaved/corrupted write?)", len(lines), n)
	}
	for _, line := range lines {
		var got Event
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line failed to parse as JSON (torn write): %v\nline: %q", err, line)
		}
	}
}
