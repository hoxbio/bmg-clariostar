package bmg

import (
	"errors"
	"net"
	"slices"
	"testing"
	"time"
)

func TestInit(t *testing.T) {
	cl, te := net.Pipe()

	c := &Clario{cl}

	fail := make(chan bool)
	go func() {
		buf := make([]byte, 13)
		_, err := te.Read(buf)
		if err != nil {
			t.Logf("error reading from file in TestInit, %s", err)
			fail <- true
		}
		te.Write([]byte{0x02, 0x00, 0x18, 0x0c, 0x01, 0x25, 0x00, 0x027, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0xc0, 0x00, 0x01, 0x36, 0x0d})
		if !slices.Equal([]byte{0x02, 0x00, 0x0d, 0x0c, 0x01, 0x00, 0x00, 0x10, 0x02, 0x00, 0x00, 0x2e, 0x0d}, buf) {
			t.Log(buf)
			fail <- true
		}
		fail <- false
	}()

	c.setup()

	select {
	case f := <-fail:
		if f {
			t.Fail()
		}
		return
	case <-time.After(time.Second * 3):
		t.Fatalf("TestInit timeout")
	}

}

func TestReadTimeout(t *testing.T) {
	cl, _ := net.Pipe()
	c := &Clario{cl}
	_, err := c.readFrame()
	if !errors.Is(err, ErrTimeout) {
		t.Fail()
	}
}
