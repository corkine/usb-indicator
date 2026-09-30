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
	Match    string `json:"match"`
	Position string `json:"position"`
	Size     int    `json:"size"`
	OffsetX  int    `json:"offset_x"`
	OffsetY  int    `json:"offset_y"`
}

const (
	defaultPosition = "top-right"
	defaultSize     = 22
	minSize         = 8
	maxSize         = 128
	defaultOffset   = 12
	maxOffset       = 10000
)

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
	cfg.Match = strings.ToUpper(strings.TrimSpace(cfg.Match))
	if cfg.Match == "" {
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
	cfg.Match = strings.ToUpper(strings.TrimSpace(cfg.Match))
	normalizeConfig(&cfg)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func normalizeConfig(cfg *config) {
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
