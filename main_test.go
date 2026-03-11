package main

import (
	"flag"
	"io"
	"testing"

	"github.com/drpaneas/devlica/internal/config"
)

func TestConfigureFlags_ExhaustiveDefaultIsFalse(t *testing.T) {
	var cfg config.Config
	var provider string
	fs := flag.NewFlagSet("devlica-test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configureFlags(fs, &cfg, &provider)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	if cfg.Exhaustive {
		t.Fatalf("expected --exhaustive default to be false")
	}
}

func TestConfigureFlags_ExhaustiveCanBeEnabled(t *testing.T) {
	var cfg config.Config
	var provider string
	fs := flag.NewFlagSet("devlica-test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	configureFlags(fs, &cfg, &provider)
	if err := fs.Parse([]string{"--exhaustive"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	if !cfg.Exhaustive {
		t.Fatalf("expected --exhaustive to enable exhaustive mode")
	}
}
