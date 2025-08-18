package bmg

import (
	"slices"
	"testing"
)

// basic sanity check on well encoding
func TestSetWells(t *testing.T) {
	rc := RunCfg{}
	rc.Plate.Rows = 8
	rc.Plate.Cols = 12
	rc.Plate.SetWells(0, 13, 26, 39)

	if !slices.Equal(rc.Plate.wells[0:5], []byte{0x80, 0x04, 0x00, 0x20, 0x01}) {
		t.Fatal("incorrect well encoding")
	}

}
