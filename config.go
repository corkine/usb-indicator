package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type config struct {
	// Keep the legacy JSON key so the original keyboard-only build can still
	// read configs saved by the local keyboard-and-mouse build.
	KeyboardMatch string `json:"match"`
	MouseMode     string `json:"mouse_mode,omitempty"`
	MouseID       string `json:"mouse_id,omitempty"`
	MouseSlot     int    `json:"mouse_slot,omitempty"`
	PollInterval  int    `json:"poll_interval_ms,omitempty"`
	Position      string `json:"position"`
	Size          int    `json:"size"`
	OffsetX       int    `json:"offset_x"`
	OffsetY       int    `json:"offset_y"`
}

const (
	defaultPosition = "top-right"
	defaultSize     = 22
	minSize         = 8
	maxSize         = 128
	defaultOffset   = 12
	maxOffset       = 10000
	defaultPollMS   = 200
)

var validPollIntervals = []int{100, 200, 500}

var validPositions = []string{
	"top-left", "top-center", "top-right",
	"center-left", "center-right",
	"bottom-left", "bottom-center", "bottom-right",
	"custom",
}

var vidPIDPattern = regexp.MustCompile(`(?i)VID_[0-9A-F]{4}&PID_[0-9A-F]{4}`)

func configPath() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(base, "usb-indicator", "config.json"), nil
}

func loadConfig() (config, error) {
	path, err := configPath()
	if err != nil {
		return config{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	var cfg config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return config{}, err
	}
	cfg.KeyboardMatch = strings.ToUpper(strings.TrimSpace(cfg.KeyboardMatch))
	if cfg.KeyboardMatch == "" {
		return config{}, errors.New("empty keyboard match rule")
	}
	normalizeConfig(&cfg)
	return cfg, nil
}

func saveConfig(cfg config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cfg.KeyboardMatch = strings.ToUpper(strings.TrimSpace(cfg.KeyboardMatch))
	normalizeConfig(&cfg)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func normalizeConfig(cfg *config) {
	cfg.KeyboardMatch = strings.ToUpper(strings.TrimSpace(cfg.KeyboardMatch))
	cfg.MouseMode = strings.ToLower(strings.TrimSpace(cfg.MouseMode))
	cfg.MouseID = strings.ToUpper(strings.TrimSpace(cfg.MouseID))
	if cfg.MouseMode != "bluetooth" && cfg.MouseMode != "unifying" {
		cfg.MouseMode = ""
		cfg.MouseID = ""
		cfg.MouseSlot = 0
	}
	if cfg.MouseMode == "unifying" && (cfg.MouseSlot < 0 || cfg.MouseSlot > 6) {
		cfg.MouseSlot = 0
	}
	if !isValidPollInterval(cfg.PollInterval) {
		cfg.PollInterval = defaultPollMS
	}
	cfg.Position = strings.ToLower(strings.TrimSpace(cfg.Position))
	if !isValidPosition(cfg.Position) {
		cfg.Position = defaultPosition
	}
	if cfg.Size < minSize || cfg.Size > maxSize {
		cfg.Size = defaultSize
	}
	if cfg.OffsetX < 0 || cfg.OffsetX > maxOffset {
		cfg.OffsetX = defaultOffset
	}
	if cfg.OffsetY < 0 || cfg.OffsetY > maxOffset {
		cfg.OffsetY = defaultOffset
	}
}

func isValidPollInterval(interval int) bool {
	for _, candidate := range validPollIntervals {
		if interval == candidate {
			return true
		}
	}
	return false
}

func isValidPosition(position string) bool {
	for _, candidate := range validPositions {
		if position == candidate {
			return true
		}
	}
	return false
}

func stableMatch(devicePath string) string {
	if value := vidPIDPattern.FindString(devicePath); value != "" {
		return strings.ToUpper(value)
	}
	return strings.ToUpper(devicePath)
}
