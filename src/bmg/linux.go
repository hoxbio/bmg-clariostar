package bmg

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// https://www.downtowndougbrown.com/2013/11/linux-custom-serial-baud-rates/
// the termios struct is the same for termios1/2 syscalls but not all fields
// will be passed along (i.e. Ispeed, Ospeed). Don't accidentally set I/Ospeed
// but make the termios IOCTLs....
// need ftdi_sio module to get serial interface to plate reader. The custom dev ID must be added.

func openPort() (*os.File, error) {
	fd, err := unix.Open("/dev/clario", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}

	// termios2 is the only version that actually supports I/O speed, otherwise its a CFLAG and I/O speed is lost
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("error getting termios2: %w", err)
	}

	// Clear Following Input flags:
	// IGNBRK - Ignore break condition
	// BRKINT - Signal interrupt on break
	// ICRNL - Map CR to NL on input
	// INLCR - Map NL to CR on input
	// PARMRK - Mark parity errors
	// INPCK Enable input parity checking
	// ISTRIP - Strip 8th bit
	// IXON - Enable software flow control
	t.Iflag &^= uint32(unix.IGNBRK | unix.BRKINT | unix.ICRNL | unix.INLCR | unix.PARMRK | unix.INPCK | unix.ISTRIP | unix.IXON)

	// Output flags
	t.Oflag = 0

	// Line processing
	// ECHO - Echo input characters
	// ECHONL - Echo newline character
	// ICANON - Canonical mode
	// IEXTEN - Implementation defined input processing
	// ISIG - When INTR, QUIT, SUSP, DSUSP are received, generate signal
	t.Lflag &^= uint32(unix.ECHO | unix.ECHONL | unix.ICANON | unix.IEXTEN | unix.ISIG)

	// Character processing
	// CSIZE - Character mask size (CS8)
	// PARENB - Enable parity generation on output and parity checking for input
	// CBAUD - Baud speed mask
	// CS8 - 8 bit character size mask
	// CREAD - Enable receiver
	// CLOCAL - Ignore modem control lines
	// BOTHER - Non standard baud rate, use Ispeed, Ospeed
	t.Cflag &^= uint32(unix.CSIZE | unix.PARENB | unix.CBAUD)
	t.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL | unix.BOTHER

	t.Ispeed = 125000
	t.Ospeed = 125000

	// Terminal special characters array
	// VMIN - Minimum number of characters for noncanonical read
	// VTIME - Timeout in deciseconds for noncanonical read
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 10

	err = unix.IoctlSetTermios(fd, unix.TCSETS2, t)
	if err != nil {
		fmt.Println(err)
		return nil, fmt.Errorf("error setting termios2: %w", err)
	}

	// discard data received and not read, and data written but not transmitted
	unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIOFLUSH)

	f := os.NewFile(uintptr(fd), "bmg")

	return f, nil
}
