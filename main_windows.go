//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
)

type keyboardChoice struct {
	MatchRule string
	Path      string
}

type mouseChoice struct {
	Mode  string
	ID    string
	Slot  int
	Label string
}

func main() {
	runtime.LockOSThread()

	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "--list":
			listKeyboards()
			return
		case "--list-mice":
			listMice()
			return
		case "--debug-unifying":
			debugUnifying()
			return
		case "--version", "-version":
			fmt.Println(version)
			return
		case "--disable":
			disableIndicator()
			return
		case "--select", "--settings":
			if err := selectAndSaveDevices(); err != nil {
				if !errors.Is(err, errSelectionCancelled) {
					showError(err)
				}
			} else {
				notifyIndicatorReload()
			}
			return
		}
	}

	release, acquired, err := acquireSingleInstance()
	if err != nil {
		showError(err)
		return
	}
	if !acquired {
		return
	}
	defer release()

	cfg, err := loadConfig()
	if err != nil {
		if err := selectAndSaveDevices(); err != nil {
			if !errors.Is(err, errSelectionCancelled) {
				showError(err)
			}
			return
		}
		cfg, err = loadConfig()
		if err != nil {
			showError(err)
			return
		}
	}
	if err := runIndicator(cfg); err != nil {
		showError(err)
	}
}

func debugUnifying() {
	hidppDebug = true
	choices, err := mouseChoices()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	for _, choice := range choices {
		if choice.Mode != "unifying" {
			continue
		}
		fmt.Printf("receiver=%s\n", choice.ID)
		slot, deviceInfo, queryErr := queryUnifyingConnection(choice.ID)
		fmt.Printf(
			"connection-query slot=%d device-info=0x%02X mouse-link=%t err=%v\n",
			slot, deviceInfo, unifyingMouseLinkEstablished(deviceInfo), queryErr,
		)
	}
}

func listMice() {
	choices, err := mouseChoices()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	for i, choice := range choices {
		state := "unknown"
		if choice.Mode == "bluetooth" {
			connected, statusErr := bluetoothMousePresent(choice.ID)
			if statusErr == nil {
				state = fmt.Sprintf("connected=%t", connected)
			} else {
				state = statusErr.Error()
			}
		} else if choice.Mode == "unifying" {
			connected, statusErr := unifyingMousePresent(choice.ID, choice.Slot, defaultPollMS)
			if statusErr == nil {
				state = fmt.Sprintf("connected=%t", connected)
			} else {
				state = statusErr.Error()
			}
		}
		fmt.Printf("[%d] %s mode=%s id=%s slot=%d %s\n", i+1, choice.Label, choice.Mode, choice.ID, choice.Slot, state)
	}
}

func listKeyboards() {
	choices, err := keyboardChoices()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	for i, choice := range choices {
		fmt.Printf("[%d] %s\n    %s\n", i+1, choice.MatchRule, choice.Path)
	}
}
