package main

import (
	"log/slog"
	"testing"
)

func TestEnvironmentParsing(t *testing.T) {
	t.Setenv("TEST_STRING", "value")
	if got := envString("TEST_STRING", "fallback"); got != "value" {
		t.Fatalf("envString=%q", got)
	}
	if got := envString("TEST_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("envString fallback=%q", got)
	}

	for _, tc := range []struct {
		name  string
		value string
		want  int64
	}{
		{name: "valid", value: "42", want: 42},
		{name: "invalid", value: "no", want: 7},
		{name: "zero", value: "0", want: 7},
		{name: "negative", value: "-1", want: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_INT", tc.value)
			if got := envInt64("TEST_INT", 7); got != tc.want {
				t.Fatalf("envInt64=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestEnvironmentLogLevel(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"invalid", slog.LevelInfo},
	} {
		t.Setenv("TEST_LEVEL", tc.value)
		if got := envLogLevel("TEST_LEVEL", slog.LevelInfo); got != tc.want {
			t.Fatalf("envLogLevel(%q)=%s want=%s", tc.value, got, tc.want)
		}
	}
}
