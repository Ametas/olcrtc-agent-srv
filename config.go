// Package main: config.go defines the on-disk YAML shape this agent reads and
// its pure translation into pkg/olcrtc/tunnel.Config. Field names deliberately
// mirror the official upstream server.yaml schema (docs/configuration.md,
// docs/settings.md in openlibrecommunity/olcrtc) rather than inventing a
// dialect — this binary generates/consumes the same shape a human could hand
// -edit for debugging on a node, and stays recognizable to anyone who has read
// the upstream docs.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/openlibrecommunity/olcrtc/pkg/olcrtc/tunnel"
	"gopkg.in/yaml.v3"
)

// Config is the root YAML document for one instance. Only `mode: srv` is
// supported — this agent is the server side only, the client (olcbox/
// owenclave/etc.) is never something we run ourselves.
type Config struct {
	Mode   string       `yaml:"mode"`
	Auth   AuthConfig   `yaml:"auth"`
	Room   RoomConfig   `yaml:"room"`
	Crypto CryptoConfig `yaml:"crypto"`
	Net    NetConfig    `yaml:"net"`

	VP8   *VP8Config   `yaml:"vp8,omitempty"`
	SEI   *SEIConfig   `yaml:"sei,omitempty"`
	Video *VideoConfig `yaml:"video,omitempty"`

	Liveness *LivenessConfig `yaml:"liveness,omitempty"`
	Traffic  *TrafficConfig  `yaml:"traffic,omitempty"`
}

type AuthConfig struct {
	Provider string `yaml:"provider"`
	Token    string `yaml:"token,omitempty"`
}

type RoomConfig struct {
	ID string `yaml:"id"`
}

type CryptoConfig struct {
	Key string `yaml:"key"`
}

type NetConfig struct {
	Transport string `yaml:"transport"`
	DNS       string `yaml:"dns,omitempty"`

	SOCKSProxyAddr string `yaml:"socks_proxy_addr,omitempty"`
	SOCKSProxyPort int    `yaml:"socks_proxy_port,omitempty"`
	SOCKSProxyUser string `yaml:"socks_proxy_user,omitempty"`
	SOCKSProxyPass string `yaml:"socks_proxy_pass,omitempty"`
}

type VP8Config struct {
	FPS       int `yaml:"fps"`
	BatchSize int `yaml:"batch_size"`
}

type SEIConfig struct {
	FPS          int `yaml:"fps"`
	BatchSize    int `yaml:"batch_size"`
	FragmentSize int `yaml:"fragment_size"`
	AckTimeoutMS int `yaml:"ack_timeout_ms"`
}

type VideoConfig struct {
	Codec      string `yaml:"codec"`
	Width      int    `yaml:"width"`
	Height     int    `yaml:"height"`
	FPS        int    `yaml:"fps"`
	QRRecovery string `yaml:"qr_recovery,omitempty"`
	QRSize     int    `yaml:"qr_size,omitempty"`
	TileModule int    `yaml:"tile_module,omitempty"`
	TileRS     int    `yaml:"tile_rs,omitempty"`
}

type LivenessConfig struct {
	IntervalSeconds int `yaml:"interval_seconds"`
	TimeoutSeconds  int `yaml:"timeout_seconds"`
	Failures        int `yaml:"failures"`
}

type TrafficConfig struct {
	MaxPayloadSize int `yaml:"max_payload_size,omitempty"`
	MinDelayMS     int `yaml:"min_delay_ms,omitempty"`
	MaxDelayMS     int `yaml:"max_delay_ms,omitempty"`
}

// LoadConfig reads and parses a YAML config file from disk.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %q: %w", path, err)
	}
	return &cfg, nil
}

// Validate checks the minimal set of fields the tunnel cannot run without.
// Deliberately conservative — extra/unknown YAML keys are not rejected here
// (yaml.Unmarshal already ignores them), only the fields this agent itself
// requires to build a working tunnel.Config.
func (c *Config) Validate() error {
	if c.Mode != "srv" {
		return fmt.Errorf("mode must be %q, got %q", "srv", c.Mode)
	}
	if c.Auth.Provider == "" {
		return fmt.Errorf("auth.provider is required")
	}
	if c.Crypto.Key == "" {
		return fmt.Errorf("crypto.key is required")
	}
	if c.Net.Transport == "" {
		return fmt.Errorf("net.transport is required")
	}
	return nil
}

// ToTunnelConfig translates the parsed YAML into a tunnel.Config. Hooks
// (AuthHook/OnSessionOpen/OnSessionClose/OnTraffic/OnHealth) are attached by
// the caller (main.go) — this function stays pure and hook-agnostic so it can
// be unit-tested without spinning up any I/O.
func (c *Config) ToTunnelConfig() tunnel.Config {
	cfg := tunnel.Config{
		Transport: c.Net.Transport,
		Provider:  c.Auth.Provider,
		RoomURL:   c.Room.ID,
		Token:     c.Auth.Token,
		KeyHex:    c.Crypto.Key,
		DNSServer: c.Net.DNS,

		SOCKSProxyAddr: c.Net.SOCKSProxyAddr,
		SOCKSProxyPort: c.Net.SOCKSProxyPort,
		SOCKSProxyUser: c.Net.SOCKSProxyUser,
		SOCKSProxyPass: c.Net.SOCKSProxyPass,
	}

	switch c.Net.Transport {
	case "vp8channel":
		if c.VP8 != nil {
			cfg.TransportOptions = tunnel.VP8Options{FPS: c.VP8.FPS, BatchSize: c.VP8.BatchSize}
		}
	case "seichannel":
		if c.SEI != nil {
			cfg.TransportOptions = tunnel.SEIOptions{
				FPS: c.SEI.FPS, BatchSize: c.SEI.BatchSize,
				FragmentSize: c.SEI.FragmentSize, AckTimeoutMS: c.SEI.AckTimeoutMS,
			}
		}
	case "videochannel":
		if c.Video != nil {
			cfg.TransportOptions = tunnel.VideoOptions{
				Width: c.Video.Width, Height: c.Video.Height, FPS: c.Video.FPS,
				QRRecovery: c.Video.QRRecovery, QRSize: c.Video.QRSize, Codec: c.Video.Codec,
				TileModule: c.Video.TileModule, TileRS: c.Video.TileRS,
			}
		}
	}

	if c.Liveness != nil {
		cfg.Liveness = tunnel.LivenessConfig{
			Interval: time.Duration(c.Liveness.IntervalSeconds) * time.Second,
			Timeout:  time.Duration(c.Liveness.TimeoutSeconds) * time.Second,
			Failures: c.Liveness.Failures,
		}
	}
	if c.Traffic != nil {
		cfg.Traffic = tunnel.TrafficConfig{
			MaxPayloadSize: c.Traffic.MaxPayloadSize,
			MinDelay:       time.Duration(c.Traffic.MinDelayMS) * time.Millisecond,
			MaxDelay:       time.Duration(c.Traffic.MaxDelayMS) * time.Millisecond,
		}
	}

	return cfg
}
