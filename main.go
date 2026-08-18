// Command olcrtc-agent-srv is route-agent's replacement for the upstream
// `olcrtc server.yaml` CLI. It embeds pkg/olcrtc/tunnel (a stable public Go
// library, not a fork — see plan doc for why this isn't the same class of
// commitment as forking sing-box) purely to get access to the OnSessionOpen/
// OnSessionClose/OnTraffic/OnHealth hooks the plain CLI has no way to expose,
// and turns them into structured JSON lines on stdout for route-agent to
// stream as telemetry (events.go). One process = one instance/room, matching
// the protocol's own one-tunnel-per-process shape (see plan doc, "Конфиг и
// топология").
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/openlibrecommunity/olcrtc/pkg/olcrtc/tunnel"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "olcrtc-agent-srv: %v\n", err)
		os.Exit(1)
	}
}

// run holds all the logic main() would otherwise have inline, so it can be
// exercised by tests without touching the real process's stdout/stderr/os.Exit.
func run(args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: olcrtc-agent-srv <config.yaml>")
	}

	cfg, err := LoadConfig(args[0])
	if err != nil {
		return err
	}

	emitter := NewEventEmitter(stdout)

	tcfg := cfg.ToTunnelConfig()
	tcfg.OnSessionOpen = emitter.OnSessionOpen
	tcfg.OnSessionClose = emitter.OnSessionClose
	tcfg.OnTraffic = emitter.OnTraffic
	tcfg.OnHealth = emitter.OnHealth
	// AuthHook deliberately left nil — device_id/claims in CLIENT_HELLO are
	// client-self-reported and unverified (see plan doc, open question #1,
	// closed 2026-08-18 by reading internal/handshake). Not wired for v1.

	srv := tunnel.New(tcfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Fprintf(stderr, "olcrtc-agent-srv: starting (provider=%s transport=%s)\n", tcfg.Provider, tcfg.Transport)
	if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
		return fmt.Errorf("tunnel run: %w", err)
	}
	return nil
}
