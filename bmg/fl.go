// in fl.go
package bmg

import (
	"encoding/binary"
	"fmt"
)

/*
TODO:
- Multichromatics (currently only 1), easy
- Plate mode (currently endpoint only), need to implement injection system to justify kinetics
- Spectral scan
- Filter based (currently only monochrometer), easy
- Time resolved
- Fluorescence Polarization
*/

// FlCfg is used to confgiure an endpoint fluorescence run
type FlCfg struct {
	Ex           int  `json:"ex"`            // excitation center wavelength
	ExBw         int  `json:"ex_bw"`         // excitation bandwidith
	Dich         int  `json:"dich"`          // dichroic wavelength * 10
	Em           int  `json:"em"`            // emission center wavelength
	EmBw         int  `json:"em_bw"`         // emission bandwidth
	Gain         int  `json:"gain"`          // gain
	FocalHeight  int  `json:"focal_height"`  // focal height (mm) * 100
	Flashes      int  `json:"flashes"`       // number of flashes 0-200, 1-3 when using FlyingMode (off by default)
	BottomOptic  bool `json:"bottom_optic"`  // use bottom optic, defaults to top optic
	SettlingTime int  `json:"settling_time"` // 0-10 deciseconds
	OrbitAvg     int  `json:"orbit_avg"`     // orbital averaging diameter (if > 0)
}

// RunFl launches a fluorescence run, blocking
func (c *Clario) RunFl(rc RunCfg, fl FlCfg) (FlData, error) {
	cmd, err := flBytes(rc, fl)
	if err != nil {
		return FlData{}, err
	}
	c.setup()
	c.waitForReady()
	c.write(cmd)
	c.waitForReady()
	resp, err := c.write(getData)
	if err != nil {
		return FlData{}, err
	}
	r, err := unmarshalFlData(resp)
	if err != nil {
		return FlData{}, err
	}
	return r, nil

}

// flBytes serializes the FlCfg and implements basic sanity checks
func flBytes(rc RunCfg, fl FlCfg) ([]byte, error) {

	// Flashes constraints
	if rc.Plate.FlyingMode {
		switch {
		case rc.Plate.Rows*rc.Plate.Cols > 96 && fl.Flashes > 1:
			return nil, fmt.Errorf("cannot do more than one flash in plate with >96 wells")
		case fl.Flashes > 3:
			return nil, fmt.Errorf("cannot do more than 3 flashes in flying mode")
		}
	}
	if fl.Flashes > 200 {
		return nil, fmt.Errorf("flashes per well must be ")
	}

	// Orbital averaging constraints
	if fl.OrbitAvg > 0 {
		switch {
		case fl.OrbitAvg > rc.Plate.WellDia/100:
			return nil, fmt.Errorf("cannot do orbital averaging > well diameter")
		case fl.Flashes > fl.OrbitAvg*17:
			return nil, fmt.Errorf("cannot do more than 17* orbital diameter flashes")
		case rc.Plate.FlyingMode:
			return nil, fmt.Errorf("cannot do orbital averaging with flying mode")
		}
	}

	cmd := make([]byte, 0, 125)

	pb, err := plateBytes(rc.Plate)
	if err != nil {
		return nil, err
	}
	cmd = append(cmd, pb...)

	var d uint8
	if fl.BottomOptic {
		d |= 1 << 6
	}
	if fl.OrbitAvg > 0 {
		d |= 1<<4 | 1<<5
	}
	cmd = append(cmd, d)

	//cmd[65:68] always zero
	cmd = append(cmd, 0x00, 0x00, 0x00)

	sb, err := shakerBytes(rc.Shake)
	if err != nil {
		return nil, err
	}
	cmd = append(cmd, sb...)

	// TODO UNKNOWN - maybe seperates optics?
	// in orbital averaging 5 bytes are inserted immedietly after here
	cmd = append(cmd, 0x27, 0x0F, 0x27, 0x0F)

	if fl.OrbitAvg > 0 {
		cmd = append(cmd, 0x03, byte(fl.OrbitAvg))
		cmd = binary.BigEndian.AppendUint16(cmd, uint16(rc.Plate.WellDia))
		cmd = append(cmd, 0x00)
	}

	if fl.SettlingTime == 0 {
		cmd = append(cmd, 1)
	} else {
		cmd = append(cmd, uint8((fl.SettlingTime*10)/2))
	}
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(fl.FocalHeight))

	// TODO multichromatic and filters
	// 0x01 seems to be number of multichromats/filters
	// when multichromats > 1 all but the last seems to have the gain and filter config
	// plus 0x00,0x04,0x00,0x03 followed by 0x00 0x00 0x00 0x0c
	cmd = append(cmd, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0c)

	cmd = binary.BigEndian.AppendUint16(cmd, uint16(fl.Gain))
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(fl.Ex*10+fl.ExBw))
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(fl.Ex*10-fl.ExBw))
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(fl.Dich))
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(fl.Em*10+fl.EmBw))
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(fl.Em*10-fl.EmBw))

	// Probably something to do with the slits on the monochrometers? differ with filter measurement
	// but not by which filter.
	cmd = append(cmd, 0x00, 0x04, 0x00, 0x03)

	if rc.PauseTime != 0 {
		cmd = append(cmd, 0x01)
	} else {
		cmd = append(cmd, 0x00)
	}
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(rc.PauseTime))

	cmd = append(cmd, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01)

	cmd = binary.BigEndian.AppendUint16(cmd, uint16(fl.Flashes))
	cmd = append(cmd, 0x00, 0x4b, 0x00, 0x00)

	return cmd, nil
}

// Fldata holds all of the known fields from the plate reader response
type FlData struct {
	Total         int      `json:"total"`         // total number of values the run will produce
	Complete      int      `json:"complete"`      // number of completed measurements
	Multichromats int      `json:"multichromats"` // number of multichromats used per well (currently 1 in fl)
	Wells         int      `json:"wells"`         // number of wells measured
	Temp          float32  `json:"temp"`          // the temperature of the incubator if enabled
	Ovf           uint32   `json:"ovf"`           // overflow value
	Vals          []uint32 `json:"vals"`          // all values measured, in row major order
}

// unmarshalFlData populates a FlData from the plate reader response bytes
func unmarshalFlData(resp []byte) (FlData, error) {

	if len(resp) < 34 {
		return FlData{}, fmt.Errorf("malformed data response, too short")
	}

	if resp[6] != 0x21 {
		return FlData{}, fmt.Errorf("incorrect data response schema for discrete fl assay")
	}

	d := FlData{}
	d.Total = int(binary.BigEndian.Uint16(resp[7:9]))
	d.Complete = int(binary.BigEndian.Uint16(resp[9:11]))
	d.Vals = make([]uint32, d.Complete)
	d.Ovf = binary.BigEndian.Uint32(resp[11:15])

	d.Multichromats = int(binary.BigEndian.Uint16(resp[16:18]))
	d.Wells = int(binary.BigEndian.Uint16(resp[18:20]))
	d.Temp = float32(binary.BigEndian.Uint16(resp[25:27]) / 10)

	for i, j := 34, 0; j < d.Complete; {

		// the resp is not large enough to contain more responses even though they are expected
		if i+4 > len(resp) {
			return FlData{}, fmt.Errorf("expected data, but received none")
		}
		d.Vals[j] = binary.BigEndian.Uint32(resp[i : i+4])

		i += 4
		j++
	}

	return d, nil

}
