//go:build windows

package main

import (
	"errors"
	"fmt"
	"strconv"
	"syscall"
	"unsafe"
)

var (
	selectorKeyboardChoices []keyboardChoice
	selectorMouseChoices    []mouseChoice
	selectorResult          config
	selectorSaved           bool
	errSelectionCancelled   = errors.New("device selection cancelled")
	procGetDlgItem          = user32.NewProc("GetDlgItem")
)

func selectAndSaveDevices() error {
	choices, err := keyboardChoices()
	if err != nil {
		return err
	}
	current, _ := loadConfig()
	normalizeConfig(&current)
	if current.KeyboardMatch != "" {
		found := false
		for _, choice := range choices {
			if choice.MatchRule == current.KeyboardMatch {
				found = true
				break
			}
		}
		if !found {
			choices = append([]keyboardChoice{{MatchRule: current.KeyboardMatch, Path: "（当前配置，设备暂不在本机）"}}, choices...)
		}
	}
	if len(choices) == 0 {
		return errors.New("没有检测到键盘设备")
	}

	mice, err := mouseChoices()
	if err != nil {
		return err
	}
	mice = append([]mouseChoice{{Label: "不启用鼠标指示"}}, mice...)
	if current.MouseMode != "" {
		found := false
		for _, choice := range mice {
			if choice.Mode == current.MouseMode && choice.ID == current.MouseID && choice.Slot == current.MouseSlot {
				found = true
				break
			}
		}
		if !found {
			mice = append(mice, mouseChoice{
				Mode: current.MouseMode, ID: current.MouseID, Slot: current.MouseSlot,
				Label: "当前鼠标配置（设备暂不在本机）",
			})
		}
	}

	selectorKeyboardChoices = choices
	selectorMouseChoices = mice
	selectorResult = config{}
	selectorSaved = false
	if err := registerClass("USBIndicatorSelector", syscall.NewCallback(selectorWndProc), 6); err != nil {
		return err
	}
	title := fmt.Sprintf("USB Keyboard & Mouse Indicator 设置 - %s", version)
	hwnd, err := createWindow(0, "USBIndicatorSelector", title, wsCaption|wsSysMenu, 200, 120, 760, 620, 0, 0)
	if err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "目标键盘", wsChild|wsVisible, 12, 12, 160, 20, hwnd, 0); err != nil {
		return err
	}
	list, err := createWindow(0, "LISTBOX", "", wsChild|wsVisible|wsVScroll|wsBorder|lbsNotify, 12, 35, 720, 175, hwnd, selectListID)
	if err != nil {
		return err
	}
	selectedKeyboard := 0
	for i, choice := range choices {
		label := choice.MatchRule + "    " + choice.Path
		procSendMessageW.Call(list, lbAddString, 0, uintptr(unsafe.Pointer(utf16(label))))
		if choice.MatchRule == current.KeyboardMatch {
			selectedKeyboard = i
		}
	}
	procSendMessageW.Call(list, lbSetCurSel, uintptr(selectedKeyboard), 0)

	if _, err = createWindow(0, "STATIC", "目标鼠标（蓝点）", wsChild|wsVisible, 12, 225, 180, 20, hwnd, 0); err != nil {
		return err
	}
	mouseList, err := createWindow(0, "LISTBOX", "", wsChild|wsVisible|wsVScroll|wsBorder|lbsNotify, 12, 248, 720, 115, hwnd, selectMouseID)
	if err != nil {
		return err
	}
	selectedMouse := 0
	for i, choice := range mice {
		label := choice.Label
		if choice.ID != "" {
			label += "    " + choice.ID
		}
		procSendMessageW.Call(mouseList, lbAddString, 0, uintptr(unsafe.Pointer(utf16(label))))
		if choice.Mode == current.MouseMode && choice.ID == current.MouseID && choice.Slot == current.MouseSlot {
			selectedMouse = i
		}
	}
	procSendMessageW.Call(mouseList, lbSetCurSel, uintptr(selectedMouse), 0)

	if _, err = createWindow(0, "STATIC", "位置", wsChild|wsVisible, 12, 390, 55, 24, hwnd, 0); err != nil {
		return err
	}
	position, err := createWindow(0, "COMBOBOX", "", wsChild|wsVisible|wsVScroll|cbsDropDownList, 70, 385, 210, 220, hwnd, selectPositionID)
	if err != nil {
		return err
	}
	positionLabels := []string{"左上", "上方居中", "右上", "左侧居中", "右侧居中", "左下", "下方居中", "右下", "自定义（左上角偏移）"}
	selectedPosition := 2
	for i, label := range positionLabels {
		procSendMessageW.Call(position, cbAddString, 0, uintptr(unsafe.Pointer(utf16(label))))
		if validPositions[i] == current.Position {
			selectedPosition = i
		}
	}
	procSendMessageW.Call(position, cbSetCurSel, uintptr(selectedPosition), 0)

	if _, err = createWindow(0, "STATIC", "检测间隔", wsChild|wsVisible, 12, 470, 70, 24, hwnd, 0); err != nil {
		return err
	}
	pollInterval, err := createWindow(0, "COMBOBOX", "", wsChild|wsVisible|wsVScroll|cbsDropDownList, 85, 465, 215, 140, hwnd, selectPollID)
	if err != nil {
		return err
	}
	pollLabels := []string{"快速（100 ms）", "平衡（200 ms，推荐）", "低频（500 ms）"}
	selectedPoll := 1
	for i, label := range pollLabels {
		procSendMessageW.Call(pollInterval, cbAddString, 0, uintptr(unsafe.Pointer(utf16(label))))
		if validPollIntervals[i] == current.PollInterval {
			selectedPoll = i
		}
	}
	procSendMessageW.Call(pollInterval, cbSetCurSel, uintptr(selectedPoll), 0)

	if _, err = createWindow(0, "STATIC", "大小（8–128 px）", wsChild|wsVisible, 320, 390, 135, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "EDIT", strconv.Itoa(current.Size), wsChild|wsVisible|wsBorder|esNumber, 460, 385, 80, 26, hwnd, selectSizeID); err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "自定义偏移：X", wsChild|wsVisible, 12, 430, 110, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "EDIT", strconv.Itoa(current.OffsetX), wsChild|wsVisible|wsBorder|esNumber, 125, 425, 80, 26, hwnd, selectOffsetXID); err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "Y", wsChild|wsVisible, 225, 430, 20, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "EDIT", strconv.Itoa(current.OffsetY), wsChild|wsVisible|wsBorder|esNumber, 250, 425, 80, 26, hwnd, selectOffsetYID); err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "px（相对工作区左上角）", wsChild|wsVisible, 342, 430, 180, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "绿点：键盘    蓝点：鼠标", wsChild|wsVisible, 340, 470, 300, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "版本："+version, wsChild|wsVisible, 12, 540, 240, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "BUTTON", "保存", wsChild|wsVisible|bsPushButton, 532, 535, 96, 30, hwnd, selectSaveID); err != nil {
		return err
	}
	if _, err = createWindow(0, "BUTTON", "取消", wsChild|wsVisible|bsPushButton, 636, 535, 96, 30, hwnd, selectCancelID); err != nil {
		return err
	}
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	if err := messageLoop(); err != nil {
		return err
	}
	if !selectorSaved {
		return errSelectionCancelled
	}
	return saveConfig(selectorResult)
}

func selectorWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmCommand:
		id := uint16(wParam & 0xffff)
		if id == selectSaveID {
			keyboardIndex, _, _ := procSendMessageW.Call(findChild(hwnd, selectListID), lbGetCurSel, 0, 0)
			mouseIndex, _, _ := procSendMessageW.Call(findChild(hwnd, selectMouseID), lbGetCurSel, 0, 0)
			positionIndex, _, _ := procSendMessageW.Call(findChild(hwnd, selectPositionID), cbGetCurSel, 0, 0)
			pollIndex, _, _ := procSendMessageW.Call(findChild(hwnd, selectPollID), cbGetCurSel, 0, 0)
			if keyboardIndex == lbErr || mouseIndex == lbErr || positionIndex == lbErr || pollIndex == lbErr || int(keyboardIndex) >= len(selectorKeyboardChoices) || int(mouseIndex) >= len(selectorMouseChoices) || int(positionIndex) >= len(validPositions) || int(pollIndex) >= len(validPollIntervals) {
				showError(errors.New("请选择键盘、鼠标、指示器位置和检测间隔"))
				return 0
			}
			size, err := readIntControl(hwnd, selectSizeID)
			if err != nil || size < minSize || size > maxSize {
				showError(fmt.Errorf("圆点大小必须在 %d 到 %d px 之间", minSize, maxSize))
				return 0
			}
			offsetX, errX := readIntControl(hwnd, selectOffsetXID)
			offsetY, errY := readIntControl(hwnd, selectOffsetYID)
			if errX != nil || errY != nil || offsetX < 0 || offsetX > maxOffset || offsetY < 0 || offsetY > maxOffset {
				showError(fmt.Errorf("X/Y 偏移必须在 0 到 %d px 之间", maxOffset))
				return 0
			}
			mouse := selectorMouseChoices[mouseIndex]
			selectorResult = config{
				KeyboardMatch: selectorKeyboardChoices[keyboardIndex].MatchRule,
				MouseMode:     mouse.Mode,
				MouseID:       mouse.ID,
				MouseSlot:     mouse.Slot,
				PollInterval:  validPollIntervals[pollIndex],
				Position:      validPositions[positionIndex],
				Size:          size,
				OffsetX:       offsetX,
				OffsetY:       offsetY,
			}
			selectorSaved = true
			procDestroyWindow.Call(hwnd)
			return 0
		}
		if id == selectCancelID {
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func findChild(parent, id uintptr) uintptr {
	hwnd, _, _ := procGetDlgItem.Call(parent, id)
	return hwnd
}

func readIntControl(parent, id uintptr) (int, error) {
	buffer := make([]uint16, 16)
	procGetWindowTextW.Call(findChild(parent, id), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return strconv.Atoi(syscall.UTF16ToString(buffer))
}

func showError(err error) {
	text := "USB Indicator: " + err.Error()
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16("USB Indicator"))), 0x10)
}
