# BMG CLARIOstar Integration

## Description
The BMG Labtech CLARIOstar integration relies on the `ftdi_sio` kernel module and ioctl with `termios2`. The instrument uses a non-standard baudrate of `125000` baud.

## Setup

```
cp bind-ftdi.sh /usr/local/bin/
chmod +x /usr/local/bin/bind-ftdi.sh
cp 99-fti.rules /etc/udev/rules.d/
```

## Usage
TBD
