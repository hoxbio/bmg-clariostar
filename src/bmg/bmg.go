// in bmg.go
package bmg

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

// |STX|Size(uint16)|NP|	Data	|Checksum(TCP)|CR|

type Clario struct {
	f io.ReadWriter
}

// Flags present in plate reader status message
type FlagID string

const (
	FlagValid         FlagID = "VALID"
	FlagBusy          FlagID = "BUSY"
	FlagRunning       FlagID = "RUNNING"
	FlagActive        FlagID = "ACTIVE"
	FlagIdle          FlagID = "IDLE"
	FlagStandby       FlagID = "STANDBY"
	FlagInitialized   FlagID = "INITIALIZED"
	FlagLidOpen       FlagID = "LID_OPEN"          // lid open
	FlagOpen          FlagID = "OPEN"              // tray sled in open position
	FlagPlateDetected FlagID = "PLATE_DETECTED"    // bit only set when loaded and plate present
	FlagZProbed       FlagID = "Z_PROBED"          // z stage crashes into plate when loaded
	FlagUnreadData    FlagID = "UNREAD_DATA"       // bit raised after run, cleared only after first read
	FlagFilterCover   FlagID = "FILTER_COVER_OPEN" // filter cover open underneath lid
)

// Flag represents a named status flag with its bitmask
type Flag struct {
	ID   FlagID
	Mask byte
	Byte int
}

// Define status flag loci
var statusFlags = []Flag{
	{FlagStandby, 1 << 1, 0}, // seems to be just a schema byte?, will most liklely change

	{FlagValid, 1 << 0, 1},
	{FlagBusy, 1 << 5, 1}, // reliable indicator of instrument status, high confidence
	{FlagRunning, 1 << 4, 1},

	{FlagUnreadData, 1, 2}, // Data is present on the plate reader that hasn't been read

	{FlagInitialized, 1 << 5, 3},   // the setup command has been previously run
	{FlagLidOpen, 1 << 6, 3},       // instrument lid is open
	{FlagOpen, 1, 3},               // plate sled is open
	{FlagPlateDetected, 1 << 1, 3}, // plate detected
	{FlagZProbed, 1 << 2, 3},       // seems to be the z stage poking the plate after loading
	{FlagActive, 1 << 3, 3},        // performing a run

	{FlagFilterCover, 1 << 6, 4},
}

// apply flag masks and return slice of raised flags
func parseStateFlags(state [5]byte) []FlagID {
	var flags []FlagID
	for _, f := range statusFlags {
		if state[f.Byte]&f.Mask != 0 {
			flags = append(flags, f.ID)
		}
	}
	return flags
}

// TODO: RE simple command schema

var initClario = []byte{0x01, 0x00, 0x00, 0x10, 0x02, 0x00}
var cmdStatus = []byte{0x80, 0x00}
var open = []byte{0x03, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00}
var close = []byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
var getData = []byte{0x05, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

// Frames data according to the BMG serial protocol
// STX | uint16 (size) | NP | data | uint16 (CS) | CR
func frame(cmd []byte) []byte {

	buf := make([]byte, len(cmd)+7)
	copy(buf, []byte{0x02, 0x00, 0x00, 0x0c})
	binary.BigEndian.PutUint16(buf[1:], uint16(len(cmd)+7))
	copy(buf[4:], cmd)

	var sum uint16
	for _, b := range buf {
		sum += uint16(b)
	}

	binary.BigEndian.PutUint16(buf[len(buf)-3:], sum)
	buf[len(buf)-1] = 0x0d

	return buf
}

func (c *Clario) write(cmd []byte) ([]byte, error) {

	buf := frame(cmd)
	_, err := c.f.Write(buf)
	if err != nil {
		return nil, err
	}
	return c.readFrame()
}

// Open connection to Clariostar
func Init() (*Clario, error) {
	f, err := openPort()
	if err != nil {
		return nil, err
	}

	c := &Clario{f}

	return c, nil
}

// infinitely polls status and prints bit fields and identified flags
// used for perturbing system state to identify single bit flags
func (c *Clario) PollState() {
	var last []byte
	for {
		time.Sleep(time.Millisecond * 100)
		resp, err := c.write(cmdStatus)
		if err != nil {
			fmt.Println(err)
		}
		if len(resp) != 17 {
			fmt.Printf("malformed status response. got %d bytes", len(resp))
		} else {
			if !slices.Equal(resp, last) {
				for i := 0; i < 6; i++ {
					fmt.Printf("%d: %08b ", i, resp[i])
				}
				fmt.Println()
				flags := parseStateFlags(([5]byte)(resp[0:5]))
				fmt.Println(flags)
			}
			last = resp

		}
	}
}

// block until the busy flag is not raised
func (c *Clario) waitForReady() error {
	var last []byte
	for {
		time.Sleep(time.Millisecond * 100)
		resp, err := c.write(cmdStatus)
		if err != nil {
			return err
		}
		if len(resp) != 17 {
			return fmt.Errorf("malformed status response. got %d bytes", len(resp))
		} else {
			if !slices.Equal(resp, last) {
				flags := parseStateFlags(([5]byte)(resp[0:5]))
				if !slices.Contains(flags, FlagBusy) {
					return nil
				}
			}
			last = resp

		}
	}
}

// Status of Clariostar, currently only implements the single bit flags
// TODO: status has many bytes set durring a run that are yet to be RE'd
// seems that spectral captures use a different schema following the bitfield
type Status struct {
	Flags []FlagID
}

// Get the status of the Clariostar
func (c *Clario) GetStatus() (Status, error) {
	s := Status{}

	resp, err := c.write(cmdStatus)
	if err != nil {
		return s, nil
	}
	if len(resp) != 17 {
		return s, fmt.Errorf("malformed status response. got %d bytes", len(resp))
	}

	s.Flags = parseStateFlags([5]byte(resp[0:5]))
	return s, nil
}

// run initialization
// moves the plate underneath optical stage and probes for presence detection
func (c *Clario) setup() error {
	_, err := c.write(initClario)
	if err != nil {
		return err
	}
	return nil
}

// Open the plate reader, blocking
func (c *Clario) Open() error {
	_, err := c.write(open)
	if err != nil {
		return err
	}
	err = c.waitForReady()
	if err != nil {
		return err
	}
	return nil

}

// Close the plate reader, blocking
func (c *Clario) Close() error {
	_, err := c.write(close)
	if err != nil {
		return err
	}
	err = c.waitForReady()
	if err != nil {
		return err
	}
	return nil
}

// Checksum (sum of header + data bytes) is incorrect
var ErrChecksumInvalid = errors.New("invalid checksum")

// Header doesn't begin with 0x02 (STX) or end with 0x0d (carriage return)
var ErrFraming = errors.New("framing errror")

// timeout in reading
var ErrTimeout = errors.New("read timout")

// read a data frame, timeout if duration has passed with os.ErrDeadlineExceeded
// resultant bytes have the header, subsequent carriage return, checksum, and termination
// byte stripped from the returned slice
//
// TODO:
// Seems like the messages sent from the plate reader all include ~5 bytes of bit flags
// following the first byte after the carraige return (schema byte?)
func (c *Clario) readFrame() ([]byte, error) {

	// Read the "header" 0x02 {len (uint16)} 0x0c
	header := make([]byte, 4)

	ch := make(chan error)
	go func() {
		_, err := io.ReadFull(c.f, header)
		if err != nil {
			ch <- fmt.Errorf("read error: %w", err)
		}
		ch <- nil
	}()

	select {
	case err := <-ch:
		if err != nil {
			return nil, err
		}
	case <-time.After(time.Second * 10):
		return nil, ErrTimeout
	}
	// validate beginning of frame
	if header[0] != 0x02 {
		return nil, errors.Join(ErrFraming, fmt.Errorf("header doesn't beging with 0x02"))
	}

	// Read the remainder of the message
	data := make([]byte, binary.BigEndian.Uint16(header[1:3])-4)
	ch = make(chan error)

	go func() {
		_, err := io.ReadFull(c.f, data)
		if err != nil {
			ch <- err
		}
		ch <- nil
	}()

	select {
	case err := <-ch:
		if err != nil {
			return nil, err
		}
	case <-time.After(time.Second * 10):
		return nil, ErrTimeout
	}

	// calculate checksum
	var sum uint16
	for i := range header {
		sum += uint16(header[i])
	}
	for i := 0; i < len(data)-3; i++ {
		sum += uint16(data[i])
	}
	cs := []byte{byte(sum >> 8), byte(sum & 0xff)}

	// check that checksum is correct
	if !slices.Equal(cs, data[len(data)-3:len(data)-1]) {
		return nil, ErrChecksumInvalid
	}

	// validate end of frame
	if data[len(data)-1] != 0x0d {
		return nil, errors.Join(ErrFraming, fmt.Errorf("message not terminated with 0x0d"))
	}

	return data[:len(data)-3], nil

}
