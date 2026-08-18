package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openlibrecommunity/olcrtc/pkg/olcrtc/tunnel"
)

func writeTempConfig(t *testing.T, yamlBody string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	if err := os.WriteFile(path, []byte(yamlBody), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadConfig_MinimalDatachannel(t *testing.T) {
	path := writeTempConfig(t, `
mode: srv
auth:
  provider: jitsi
room:
  id: "https://meet.jit.si/testroom"
crypto:
  key: "deadbeef"
net:
  transport: datachannel
  dns: "8.8.8.8:53"
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Auth.Provider != "jitsi" {
		t.Errorf("provider = %q, want jitsi", cfg.Auth.Provider)
	}
	if cfg.Net.Transport != "datachannel" {
		t.Errorf("transport = %q, want datachannel", cfg.Net.Transport)
	}

	tcfg := cfg.ToTunnelConfig()
	if tcfg.Provider != "jitsi" || tcfg.Transport != "datachannel" {
		t.Errorf("ToTunnelConfig mismatch: %+v", tcfg)
	}
	if tcfg.RoomURL != "https://meet.jit.si/testroom" {
		t.Errorf("RoomURL = %q", tcfg.RoomURL)
	}
	if tcfg.KeyHex != "deadbeef" {
		t.Errorf("KeyHex = %q", tcfg.KeyHex)
	}
	if tcfg.TransportOptions != nil {
		t.Errorf("datachannel should carry no TransportOptions, got %#v", tcfg.TransportOptions)
	}
}

func TestLoadConfig_VP8OptionsCarried(t *testing.T) {
	path := writeTempConfig(t, `
mode: srv
auth:
  provider: wbstream
room:
  id: "room-01"
crypto:
  key: "abc123"
net:
  transport: vp8channel
vp8:
  fps: 60
  batch_size: 64
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	tcfg := cfg.ToTunnelConfig()
	opts, ok := tcfg.TransportOptions.(tunnel.VP8Options)
	if !ok {
		t.Fatalf("TransportOptions = %#v, want tunnel.VP8Options", tcfg.TransportOptions)
	}
	if opts.FPS != 60 || opts.BatchSize != 64 {
		t.Errorf("VP8Options = %+v, want {FPS:60 BatchSize:64}", opts)
	}
}

func TestLoadConfig_RejectsWrongMode(t *testing.T) {
	path := writeTempConfig(t, `
mode: cnc
auth:
  provider: jitsi
room:
  id: "room"
crypto:
  key: "abc"
net:
  transport: datachannel
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for mode: cnc, got nil")
	}
}

func TestLoadConfig_RejectsMissingRequiredFields(t *testing.T) {
	cases := map[string]string{
		"missing provider": `
mode: srv
room: {id: "x"}
crypto: {key: "abc"}
net: {transport: datachannel}
`,
		"missing crypto key": `
mode: srv
auth: {provider: jitsi}
room: {id: "x"}
net: {transport: datachannel}
`,
		"missing transport": `
mode: srv
auth: {provider: jitsi}
room: {id: "x"}
crypto: {key: "abc"}
net: {}
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeTempConfig(t, body)
			if _, err := LoadConfig(path); err == nil {
				t.Fatalf("expected validation error for case %q, got nil", name)
			}
		})
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestToTunnelConfig_LivenessAndTrafficConverted(t *testing.T) {
	path := writeTempConfig(t, `
mode: srv
auth: {provider: jitsi}
room: {id: "room"}
crypto: {key: "abc"}
net: {transport: datachannel}
liveness:
  interval_seconds: 10
  timeout_seconds: 15
  failures: 4
traffic:
  max_payload_size: 4096
  min_delay_ms: 5
  max_delay_ms: 30
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	tcfg := cfg.ToTunnelConfig()
	if tcfg.Liveness.Interval != 10*time.Second {
		t.Errorf("Liveness.Interval = %v, want 10s", tcfg.Liveness.Interval)
	}
	if tcfg.Liveness.Timeout != 15*time.Second {
		t.Errorf("Liveness.Timeout = %v, want 15s", tcfg.Liveness.Timeout)
	}
	if tcfg.Liveness.Failures != 4 {
		t.Errorf("Liveness.Failures = %d, want 4", tcfg.Liveness.Failures)
	}
	if tcfg.Traffic.MaxPayloadSize != 4096 {
		t.Errorf("Traffic.MaxPayloadSize = %d, want 4096", tcfg.Traffic.MaxPayloadSize)
	}
	if tcfg.Traffic.MinDelay != 5*time.Millisecond {
		t.Errorf("Traffic.MinDelay = %v, want 5ms", tcfg.Traffic.MinDelay)
	}
	if tcfg.Traffic.MaxDelay != 30*time.Millisecond {
		t.Errorf("Traffic.MaxDelay = %v, want 30ms", tcfg.Traffic.MaxDelay)
	}
}
