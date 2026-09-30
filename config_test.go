package main

import (
	"path/filepath"
	"testing"
)

func TestStableMatch(t *testing.T) {
	got := stableMatch(`\\?\hid#vid_046d&pid_c33c&mi_00#7&123&0&0000`)
	if want := "VID_046D&PID_C33C"; got != want {
		t.Fatalf("stableMatch() = %q, want %q", got, want)
	}
}

func TestConfigPathUsesLocalAppData(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\test\AppData\Local`)
	got, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(`C:\Users\test\AppData\Local`, "usb-indicator", "config.json")
	if got != want {
		t.Fatalf("configPath() = %q, want %q", got, want)
	}
}

func TestNormalizeConfigDefaults(t *testing.T) {
	cfg := config{KeyboardMatch: "VID_1234&PID_5678", OffsetX: -1, OffsetY: maxOffset + 1}
	normalizeConfig(&cfg)
	if cfg.Position != defaultPosition || cfg.Size != defaultSize || cfg.OffsetX != defaultOffset || cfg.OffsetY != defaultOffset || cfg.PollInterval != defaultPollMS {
		t.Fatalf("normalizeConfig() = %+v", cfg)
	}
}

func TestNormalizeConfigPollInterval(t *testing.T) {
	for _, interval := range validPollIntervals {
		cfg := config{PollInterval: interval}
		normalizeConfig(&cfg)
		if cfg.PollInterval != interval {
			t.Fatalf("normalizeConfig() interval = %d, want %d", cfg.PollInterval, interval)
		}
	}

	cfg := config{PollInterval: 123}
	normalizeConfig(&cfg)
	if cfg.PollInterval != defaultPollMS {
		t.Fatalf("normalizeConfig() invalid interval = %d, want %d", cfg.PollInterval, defaultPollMS)
	}
}

func TestNormalizeConfigMouse(t *testing.T) {
	cfg := config{MouseMode: " BLUETOOTH ", MouseID: "c5f631a98e2a"}
	normalizeConfig(&cfg)
	if cfg.MouseMode != "bluetooth" || cfg.MouseID != "C5F631A98E2A" {
		t.Fatalf("normalizeConfig() mouse = %q/%q", cfg.MouseMode, cfg.MouseID)
	}

	cfg = config{MouseMode: "unifying", MouseID: "vid_046d&pid_c52b", MouseSlot: 9}
	normalizeConfig(&cfg)
	if cfg.MouseSlot != 0 {
		t.Fatalf("normalizeConfig() slot = %d, want 0", cfg.MouseSlot)
	}
}
