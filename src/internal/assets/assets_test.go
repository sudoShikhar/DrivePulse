package assets

import (
	"bytes"
	"testing"
)

func TestEmbeddedIconsIntegrity(t *testing.T) {
	if len(IconActiveICO) == 0 || len(IconActivePNG) == 0 {
		t.Errorf("expected active icons to be non-empty")
	}
	if len(IconDisabledICO) == 0 || len(IconDisabledPNG) == 0 {
		t.Errorf("expected disabled icons to be non-empty")
	}
	if len(IconWarningICO) == 0 || len(IconWarningPNG) == 0 {
		t.Errorf("expected warning icons to be non-empty")
	}

	pngMagic := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if !bytes.HasPrefix(IconActivePNG, pngMagic) {
		t.Errorf("iconActivePNG has invalid PNG magic bytes")
	}
	if !bytes.HasPrefix(IconDisabledPNG, pngMagic) {
		t.Errorf("iconDisabledPNG has invalid PNG magic bytes")
	}
	if !bytes.HasPrefix(IconWarningPNG, pngMagic) {
		t.Errorf("iconWarningPNG has invalid PNG magic bytes")
	}

	icoMagic := []byte{0x00, 0x00, 0x01, 0x00}
	if !bytes.HasPrefix(IconActiveICO, icoMagic) {
		t.Errorf("iconActiveICO has invalid ICO magic bytes")
	}
	if !bytes.HasPrefix(IconDisabledICO, icoMagic) {
		t.Errorf("iconDisabledICO has invalid ICO magic bytes")
	}
	if !bytes.HasPrefix(IconWarningICO, icoMagic) {
		t.Errorf("iconWarningICO has invalid ICO magic bytes")
	}

	if len(GetActiveIcon()) == 0 {
		t.Errorf("getActiveIcon() returned empty slice")
	}
	if len(GetDisabledIcon()) == 0 {
		t.Errorf("getDisabledIcon() returned empty slice")
	}
	if len(GetWarningIcon()) == 0 {
		t.Errorf("getWarningIcon() returned empty slice")
	}
}
