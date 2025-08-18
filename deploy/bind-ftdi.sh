#!/bin/bash
# copy to /usr/local/bin/bind-ftdi.sh

# load driver
modprobe ftdi_sio

# register device (not done automatically)
echo 0403 bb68 > /sys/bus/usb-serial/drivers/ftdi_sio/new_id