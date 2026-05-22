package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlainMarkers(t *testing.T) {
	cases := map[string]struct {
		got  string
		want string
	}{
		"OK":    {OK(true), "[OK]"},
		"Warn":  {Warn(true), "[WARN]"},
		"Err":   {Err(true), "[ERR]"},
		"Info":  {Info(true), "[INFO]"},
		"Arrow": {Arrow(true), "->"},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s plain: got %q want %q", name, c.got, c.want)
		}
	}
}

func TestPlainTextHelpersPassThrough(t *testing.T) {
	const s = "hello"
	for _, fn := range []func(bool, string) string{Title, Header, Dim} {
		if got := fn(true, s); got != s {
			t.Errorf("plain text helper modified input: got %q want %q", got, s)
		}
	}
}

func TestFancyMarkersNonEmptyAndContainGlyph(t *testing.T) {
	cases := []struct {
		name  string
		got   string
		glyph string
	}{
		{"OK", OK(false), "✓"},
		{"Warn", Warn(false), "⚠"},
		{"Err", Err(false), "✗"},
		{"Info", Info(false), "ℹ"},
		{"Arrow", Arrow(false), "→"},
	}
	for _, c := range cases {
		if c.got == "" {
			t.Errorf("%s fancy: empty output", c.name)
		}
		if !strings.Contains(c.got, c.glyph) {
			t.Errorf("%s fancy: output %q missing glyph %q", c.name, c.got, c.glyph)
		}
	}
}

func TestStatusBadgePlain(t *testing.T) {
	cases := []string{"open", "draft", "open/blocked", "merged", "closed", "unknown", "error", "weird-label"}
	for _, label := range cases {
		if got := StatusBadge(true, label); got != label {
			t.Errorf("plain badge for %q: got %q want %q", label, got, label)
		}
	}
}

func TestStatusBadgeFancyContainsLabel(t *testing.T) {
	labels := []string{"open", "draft", "open/blocked", "merged", "closed", "unknown"}
	for _, label := range labels {
		got := StatusBadge(false, label)
		if !strings.Contains(got, label) {
			t.Errorf("fancy badge for %q missing label: %q", label, got)
		}
	}
}

func TestStatusBadgeFancyUnknownLabelPassesThrough(t *testing.T) {
	const label = "totally-custom"
	if got := StatusBadge(false, label); got != label {
		t.Errorf("default branch should return label verbatim: got %q want %q", got, label)
	}
}

func TestIsPlainExplicitFlag(t *testing.T) {
	prev := Plain
	t.Cleanup(func() { Plain = prev })

	Plain = true
	if !IsPlain(&bytes.Buffer{}) {
		t.Fatal("IsPlain should be true when Plain flag set")
	}
}

func TestIsPlainNoColorEnv(t *testing.T) {
	prev := Plain
	t.Cleanup(func() { Plain = prev })
	Plain = false

	t.Setenv("NO_COLOR", "1")
	if !IsPlain(&bytes.Buffer{}) {
		t.Fatal("IsPlain should be true when NO_COLOR is set")
	}
}

func TestIsPlainNonTTYWriter(t *testing.T) {
	prev := Plain
	t.Cleanup(func() { Plain = prev })
	Plain = false
	t.Setenv("NO_COLOR", "")

	if !IsPlain(&bytes.Buffer{}) {
		t.Fatal("IsPlain should be true for non-*os.File writers")
	}
}
