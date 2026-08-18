# olcrtc-agent-srv

`route-agent`'s replacement for the upstream `olcrtc server.yaml` CLI (`mode: srv`). Embeds
[`pkg/olcrtc/tunnel`](https://github.com/openlibrecommunity/olcrtc/tree/master/pkg/olcrtc/tunnel)
(a stable public Go library dependency — **not a fork**, no vendored/patched upstream source) purely
to get access to the `OnSessionOpen`/`OnSessionClose`/`OnTraffic`/`OnHealth` hooks the plain CLI has
no way to expose, and turns them into structured JSON lines on stdout for `route-agent` to stream as
telemetry.

Design/rationale: `C:\Users\Ametas\.claude\plans\olcrtc-redesign.md` (route-orchestrator project),
section "Новая архитектура: собственный слой".

## Why this exists instead of `Oleglog/Olcrtc_manager`

`Olcrtc_manager` (the third-party admin REST API some deployments use) exposes only a coarse
`active_peers` snapshot via polling and no byte-level traffic counters at all. `pkg/olcrtc/tunnel`'s
public hooks give real-time session-open/close events and per-session `bytesIn`/`bytesOut` — a much
better signal for the quota/abuse-detection telemetry route-orchestrator needs, without depending on
an undocumented third-party wire dialect (verified live: its `uri`/`subscription_uri` fields use a
diaslect that doesn't match the protocol's own documented `docs/uri.md` convention).

## Usage

```
olcrtc-agent-srv <config.yaml>
```

One process = one instance/room — the protocol itself doesn't multiplex multiple rooms in a single
process (unlike sing-box/xray), so this matches a per-user systemd unit 1:1, same shape a
third-party manager would also need under the hood.

### Config

YAML field names deliberately mirror the official upstream schema
(`docs/configuration.md`/`docs/settings.md` in `openlibrecommunity/olcrtc`) rather than inventing a
dialect. Minimal example:

```yaml
mode: srv
auth:
  provider: jitsi
room:
  id: "https://meet.example.org/some-room"
crypto:
  key: "<64 hex chars>"
net:
  transport: datachannel
  dns: "8.8.8.8:53"
```

See `docs/settings.md` upstream for `vp8`/`sei`/`video` transport option blocks, `liveness`, and
`traffic` pacing fields — same names, this repo's `config.go` maps them 1:1 into
`tunnel.Config`/`tunnel.LivenessConfig`/`tunnel.TrafficConfig`.

### Telemetry output (stdout)

One JSON object per line:

```json
{"type":"session_open","ts":"...","session_id":"...","device_id":"...","claims":{...}}
{"type":"session_close","ts":"...","session_id":"...","reason":"..."}
{"type":"traffic","ts":"...","session_id":"...","addr":"...","bytes_in":1024,"bytes_out":2048}
{"type":"health","ts":"...","health":{...}}
```

`device_id`/`claims` in `session_open` are **client-self-reported, not cryptographically verified**
(confirmed by reading `internal/handshake` upstream) — do not treat them as an authenticated identity
signal. `session_id` is server-generated and trustworthy; use it (not `device_id`) for concurrent
-session/abuse-detection logic.

`AuthHook` is deliberately not wired for the same reason — see the plan doc's "Открытые вопросы" for
the full reasoning.

## Build

```
go build .
```

Go 1.26+ (matches upstream's own requirement). No CGO, no forked/vendored upstream source — `go.mod`
pins a normal module dependency on `github.com/openlibrecommunity/olcrtc`.

## Test

```
go test ./...
```

Unit tests cover config parsing/validation/translation and the event emitter (including a
concurrent-write atomicity guard, since the tunnel invokes hooks from per-session goroutines writing
to a shared stdout stream). They do not exercise a real `tunnel.Run()` — that needs live
provider/network connectivity and belongs to the plan's live-verification step, not `go test`.

Live smoke-tested manually (2026-08-18) against a real self-hosted Jitsi instance
(`meet.egovm.ru`, from upstream's `docs/jitsi.instances.yaml`) — connected, joined the MUC, and sat
waiting for a peer as expected. Public `meet.jit.si` currently rejects anonymous joins with
`token required` (SASL failure) — contradicts the upstream docs' "no registration required" claim,
apparently a newer anti-abuse policy on Jitsi's side. Use an instance from `jitsi.instances.yaml`,
not the public default, until/unless that's rechecked.
