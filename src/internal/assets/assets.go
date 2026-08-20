package assets

import (
	_ "embed"
	"runtime"
)

//go:embed icons/icon_active.ico
var IconActiveICO []byte

//go:embed icons/icon_active.png
var IconActivePNG []byte

//go:embed icons/icon_disabled.ico
var IconDisabledICO []byte

//go:embed icons/icon_disabled.png
var IconDisabledPNG []byte

//go:embed icons/icon_warning.ico
var IconWarningICO []byte

//go:embed icons/icon_warning.png
var IconWarningPNG []byte

func GetActiveIcon() []byte {
	if runtime.GOOS == "windows" {
		return IconActiveICO
	}
	return IconActivePNG
}

func GetDisabledIcon() []byte {
	if runtime.GOOS == "windows" {
		return IconDisabledICO
	}
	return IconDisabledPNG
}

func GetWarningIcon() []byte {
	if runtime.GOOS == "windows" {
		return IconWarningICO
	}
	return IconWarningPNG
}
