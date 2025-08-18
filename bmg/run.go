package bmg

import (
	"encoding/binary"
	"fmt"
)

/*
TODO:
- ACU (atmospheric control unit)
	I don't think this is ever put into the run command, if so, implement elsewhere
- Injection controller
- Stacker?
*/

// RunCfg holds plate reading modality agnostic configuration options for the run
type RunCfg struct {
	Plate     PlateCfg
	Shake     ShakerCfg
	PauseTime int // time (in seconds to pause)
}

// PlateCfg holds the configuration for plate geometry and measurement ordering, timing
type PlateCfg struct {
	Length      int      // plate length(mm) * 100, defaults to 127.76mm
	Width       int      // plate length(mm) * 100, defaults to 85.48mm
	CornerX     int      // mm * 100 top left corner to center of well 0 along length(x)
	CornerY     int      // mm * 100 top left corner to center of well 0 along width(y)
	WellDia     int      // well diameter(mm) * 100
	Cols        int      // number of columns in plate
	Rows        int      // number of rows in plate
	wells       [48]byte // set only by setWells, defaults to all wells
	wSet        bool     // internal use, wells have been set.
	StartCorner Corner   // which corner to begin measurements from
	Uni         bool     // read wells in only one direction then return to origin edge
	Vert        bool     // read vertically
	FlyingMode  bool     // keeps stage moving, measures over well center, default off, not for abs

}
type Corner uint8

const (
	TopLeft     Corner = 0b0001
	TopRight    Corner = 0b0011
	BottomLeft  Corner = 0b0101
	BottomRight Corner = 0b0111
)

// SetWells changes the default behavior of a PlateCfg from reading all wells in a plate to
// reading the wells specified by idx. Idx is zero-based indexed in row major order.
//
// the plate encoding is 48 bytes (384 bit) where the bits encode if each well is going to
// be read in row major order from byte 0 to byte 48 and bit 7 to bit 0. The first well is
// the MSB of the first byte and the 8th well is the LSB of the first byte.
func (p *PlateCfg) SetWells(idx ...int) error {
	if p.Rows == 0 || p.Cols == 0 {
		return fmt.Errorf("row and column count must be set")
	}
	for _, v := range idx {
		p.wells[v/p.Rows] |= (1 << (7 - v%8))
	}
	p.wSet = true
	return nil
}

// plateBytes serializes the plate configuration
func plateBytes(pl PlateCfg) ([]byte, error) {
	switch {
	case pl.Length == 0, pl.Width == 0, pl.CornerX == 0, pl.CornerY == 0:
		return nil, fmt.Errorf("must set plate parameters")
	}

	cmd := make([]byte, 0, 63)

	cmd = append(cmd, 0x04)

	// ensure constructor defaults these
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(pl.Length))
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(pl.Width))

	cmd = binary.BigEndian.AppendUint16(cmd, uint16(pl.CornerX))
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(pl.CornerY))

	// Calculate the dimx,y
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(pl.Length-pl.CornerX))
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(pl.Width-pl.CornerY))
	cmd = append(cmd, byte(pl.Cols), byte(pl.Rows))

	cmd = append(cmd, pl.wells[:]...)
	if !pl.wSet {
		// read all wells in the plate
		for i := 0; i < (pl.Cols*pl.Rows)/8; i++ {
			pl.wells[i] = 0xff
		}
	}

	// Set the scanning mode
	var d uint8
	// | uni-directional | start corner (3) | vertical/horizontal | flying mode | always set | 0 |
	if pl.Uni {
		d |= 1 << 7
	}
	d |= uint8(pl.StartCorner<<4) & 0x70
	if pl.Vert {
		d |= 1 << 3
	}
	if pl.FlyingMode {
		d |= 1 << 2
	}
	d |= 1 << 1 // UNKNOWN BIT ALWAYS SET, discrete?

	cmd = append(cmd, d)

	return cmd, nil

}

type ShakeType int
type ShakeSpeed int

const (
	ShakeOrbital ShakeType = iota
	ShakeLinear
	ShakeDoubleOrbital
	ShakeMeander
)
const (
	Shake100 ShakeSpeed = iota // valid shake speeds are only 1-7 * 100 RPM
	Shake200
	Shake300
	Shake400
	Shake500
	Shake600
	Shake700
)

// ShakerCfg is used to configure the 'shaker' (xy stage) of the plate reader
// at this point in time it is only used for shake-before functionality
type ShakerCfg struct {
	Shake    ShakeType  // what form of shaking
	Speed    ShakeSpeed // what speed to shake
	Duration int        // how long to shake (in seconds)
}

// ShakerBytes serializes the shaker configuration
func shakerBytes(sh ShakerCfg) ([]byte, error) {
	if sh.Shake == ShakeMeander && int(sh.Speed) > 2 {
		return nil, fmt.Errorf("cannot do meander shake at > 300rpm")
	}
	cmd := make([]byte, 4)
	if sh.Duration != 0 {
		cmd[0] = 1<<4 | uint8(sh.Shake)
		cmd[1] = byte(sh.Speed)
		binary.BigEndian.PutUint16(cmd[2:4], uint16(sh.Duration))
	}
	return cmd, nil
}
