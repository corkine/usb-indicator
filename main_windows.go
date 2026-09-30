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
	Match string
	Path  string
}

func main() {
	runtime.LockOSThread()

	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "--list":
			listKeyboards()
			return
		case "--version", "-version":
			fmt.Println(version)
			return
		case "--disable":
			disableIndicator()
			return
		case "--select", "--settings":
			if err := selectAndSaveKeyboard(); err != nil {
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
		if err := selectAndSaveKeyboard(); err != nil {
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

func listKeyboards() {
	choices, err := keyboardChoices()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	for i, choice := range choices {
		fmt.Printf("[%d] %s\n    %s\n", i+1, choice.Match, choice.Path)
	}
}
