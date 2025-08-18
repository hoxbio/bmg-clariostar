package bmg

import (
	"encoding/binary"
	"fmt"
)

/*
TODO:
- Spextral scan (discrete only currently), need better understanding of spectral data schema,
	seems to be the only mode in which data must be read through multiple commands
	(MCU memory limitation prob)
*/

// DiscreteAbs holds the configuration for a discrete absorbance assay
type DiscreteAbs struct {
	Wavelengths  []int // discrete points to measure(nm, 200-1000), must be of length 1-8
	Flashes      int   // number of flashes 0-200
	SettlingTime int   // 0-10 deciseconds
}

// DiscreteAbsData holds all of the known fields from the plate reader response
type DiscreteAbsData struct {
	Total        int         // total number of values the run will produce
	Complete     int         // number of completed measurements
	Wavelengths  int         // number of multichromats used per well (currently only supporting uniform)
	Wells        int         // number of wells measured
	Temp         float32     // the temperature of the incubator if enabled
	Ovf          uint32      // overflow value
	Transmission [][]float32 // % transmission values, [well][wavelength] wells are row major order
}

// RunAbsDiscrete runs DiscreteAbs, blocking
func (c *Clario) RunAbsDiscrete(rc RunCfg, abs DiscreteAbs) (DiscreteAbsData, error) {
	cmd, err := absDiscreteBytes(rc, abs)
	if err != nil {
		return DiscreteAbsData{}, err
	}
	c.setup()
	c.waitForReady()
	c.write(cmd)
	c.waitForReady()
	resp, err := c.write(getData)
	if err != nil {
		return DiscreteAbsData{}, err
	}
	r, err := unmarshalAbsData(resp)
	if err != nil {
		return DiscreteAbsData{}, err
	}
	return r, nil

}

// absDiscreteBytes serializes the run command, implements sanity checks
func absDiscreteBytes(rc RunCfg, abs DiscreteAbs) ([]byte, error) {
	// sanity checks
	if l := len(abs.Wavelengths); l == 0 || l > 8 {
		return nil, fmt.Errorf("invalid number of wavelengths (must be 1-8)")
	}
	if abs.SettlingTime > 10 {
		return nil, fmt.Errorf("settling time too high, must be 0-10")
	}
	for _, w := range abs.Wavelengths {
		if w < 200 || w > 1000 {
			return nil, fmt.Errorf("invalid wavelength in Wavelengths")
		}
	}
	if rc.Plate.FlyingMode {
		return nil, fmt.Errorf("flying mode not valid for absorbance")
	}

	cmd := make([]byte, 0, 111+len(abs.Wavelengths)*2)
	pb, err := plateBytes(rc.Plate)
	if err != nil {
		return nil, err
	}
	cmd = append(cmd, pb...)
	// absorbance specific? This is normally optic + orbitavg bit flags
	cmd = append(cmd, 0x02)

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

	cmd = append(cmd, 0x19, uint8(len(abs.Wavelengths)))
	for _, v := range abs.Wavelengths {
		cmd = binary.BigEndian.AppendUint16(cmd, uint16(v*10))
	}

	// TODO UNKNOWN
	cmd = append(cmd, 0x00, 0x00, 0x00, 0x64, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x00)

	if rc.PauseTime != 0 {
		cmd = append(cmd, 0x01)
	} else {
		cmd = append(cmd, 0x00)
	}
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(rc.PauseTime))

	cmd = append(cmd, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01)
	cmd = binary.BigEndian.AppendUint16(cmd, uint16(abs.Flashes))

	cmd = append(cmd, 0x00, 0x01, 0x00, 0x00)

	return cmd, nil
}

// chromat holds the ADC range for a given chromat
type chromat struct {
	valHi float32
	valLo float32
}

// unmarshalAbsData returns a DiscreteAbsData populated with known fields from plate reader response
func unmarshalAbsData(resp []byte) (DiscreteAbsData, error) {

	if len(resp) < 34 {
		return DiscreteAbsData{}, fmt.Errorf("malformed data response, too short")
	}

	if resp[6] != 0x29 {
		return DiscreteAbsData{}, fmt.Errorf("incorrect data response schema for discrete abs assay")
	}

	d := DiscreteAbsData{}
	d.Total = int(binary.BigEndian.Uint16(resp[7:9]))
	d.Complete = int(binary.BigEndian.Uint16(resp[9:11]))

	d.Ovf = binary.BigEndian.Uint32(resp[11:15])
	// unknown 15
	d.Wavelengths = int(binary.BigEndian.Uint16(resp[16:18]))
	// unknown 18,19
	d.Wells = int(binary.BigEndian.Uint16(resp[20:22]))
	// unknown 22
	d.Temp = float32(binary.BigEndian.Uint16(resp[23:25]) / 10)
	// unknown 25-31

	// raw well reads
	vals := make([]float32, d.Wells*d.Wavelengths)
	var i, j = 36, 0
	for j < len(vals) {
		if i+4 > len(resp) {
			return DiscreteAbsData{}, fmt.Errorf("expected more data")
		}
		vals[j] = float32(binary.BigEndian.Uint32(resp[i : i+4]))

		i += 4
		j++
	}

	// well reference reads
	ref := make([]float32, d.Wells)
	j = 0
	for j < len(ref) {
		if i+4 > len(resp) {
			return DiscreteAbsData{}, fmt.Errorf("expected more data")
		}
		ref[j] = float32(binary.BigEndian.Uint32(resp[i : i+4]))

		i += 4
		j++
	}

	// chromat reference reads
	chromats := make([]chromat, d.Wavelengths)
	j = 0
	for j < len(chromats) {
		if i+8 > len(resp) {
			return DiscreteAbsData{}, fmt.Errorf("expected more data")
		}
		chromats[j].valHi = float32(binary.BigEndian.Uint32(resp[i : i+4]))
		chromats[j].valLo = float32(binary.BigEndian.Uint32(resp[i+4 : i+8]))
		i += 8
		j++
	}

	// reference channel reads
	if i+8 > len(resp) {
		return DiscreteAbsData{}, fmt.Errorf("expected more data")
	}
	refChanHi := float32(binary.BigEndian.Uint32(resp[i : i+4]))
	refChanLo := float32(binary.BigEndian.Uint32(resp[i+4 : i+8]))

	// calculate transmission per well,wavelength
	d.Transmission = make([][]float32, d.Wells)
	for i := range d.Transmission {
		// calculate the normalized well reference value through min max normalization
		// against the reference channel reading
		d.Transmission[i] = make([]float32, d.Wavelengths)
		wref := (ref[i] - refChanLo) / (refChanHi - refChanLo)

		for j := range d.Transmission[i] {
			// calculate the normalized sample reading normalized against the chromat
			value := (vals[i+(j*d.Wells)] - chromats[j].valLo) / (chromats[j].valHi - chromats[j].valLo)
			d.Transmission[i][j] = value / wref * 100
		}
	}
	return d, nil

}
