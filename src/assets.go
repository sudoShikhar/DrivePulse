package main

import (
	_ "embed"
	"runtime"
)

//go:embed assets/icon_active.ico
var iconActiveICO []byte

//go:embed assets/icon_active.png
var iconActivePNG []byte

//go:embed assets/icon_disabled.ico
var iconDisabledICO []byte

//go:embed assets/icon_disabled.png
var iconDisabledPNG []byte

//go:embed assets/icon_warning.ico
var iconWarningICO []byte

//go:embed assets/icon_warning.png
var iconWarningPNG []byte

func getActiveIcon() []byte {
	if runtime.GOOS == "windows" {
		return iconActiveICO
	}
	return iconActivePNG
}

func getDisabledIcon() []byte {
	if runtime.GOOS == "windows" {
		return iconDisabledICO
	}
	return iconDisabledPNG
}

func getWarningIcon() []byte {
	if runtime.GOOS == "windows" {
		return iconWarningICO
	}
	return iconWarningPNG
}
