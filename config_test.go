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
	cfg := config{Match: "VID_1234&PID_5678", OffsetX: -1, OffsetY: maxOffset + 1}
	normalizeConfig(&cfg)
	if cfg.Position != defaultPosition || cfg.Size != defaultSize || cfg.OffsetX != defaultOffset || cfg.OffsetY != defaultOffset {
		t.Fatalf("normalizeConfig() = %+v", cfg)
	}
}
