package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

// These only exercise the argument-parsing/config-loading error paths — a
// real srv.Run(ctx) needs actual network/provider connectivity and belongs to
// the live verification step in the plan doc, not a unit test.

func TestRun_WrongArgCount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run(nil, &stdout, &stderr); err == nil {
		t.Fatal("expected usage error for zero args, got nil")
	}
	if err := run([]string{"a", "b"}, &stdout, &stderr); err == nil {
		t.Fatal("expected usage error for two args, got nil")
	}
}

func TestRun_PropagatesConfigLoadError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if err := run([]string{missing}, &stdout, &stderr); err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
}
