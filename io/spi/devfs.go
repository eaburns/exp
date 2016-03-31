// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spi

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/exp/io/spi/driver"
)

const (
	devfs_MAGIC = 107

	devfs_NRBITS   = 8
	devfs_TYPEBITS = 8
	devfs_SIZEBITS = 13
	devfs_DIRBITS  = 3

	devfs_NRSHIFT   = 0
	devfs_TYPESHIFT = devfs_NRSHIFT + devfs_NRBITS
	devfs_SIZESHIFT = devfs_TYPESHIFT + devfs_TYPEBITS
	devfs_DIRSHIFT  = devfs_SIZESHIFT + devfs_SIZEBITS

	devfs_READ  = 2
	devfs_WRITE = 4
)

type payload struct {
	tx       uint64
	rx       uint64
	length   uint32
	speed    uint32
	delay    uint16
	bits     uint8
	csChange uint8
	txNBits  uint8
	rxNBits  uint8
	pad      uint16
}

// DevFS is an SPI driver that works against the devfs.
type DevFS struct{}

// Open opens /dev/spidev<bus>.<chip> and returns a connection.
func (d *DevFS) Open(bus, chip int) (driver.Conn, error) {
	n := fmt.Sprintf("/dev/spidev%d.%d", bus, chip)
	f, err := os.OpenFile(n, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &devfsConn{f: f}, nil
}

type devfsConn struct {
	f     *os.File
	mode  uint8
	speed uint32
	bits  uint8
}

func (c *devfsConn) Configure(k, v int) error {
	switch k {
	case driver.Mode:
		m := uint8(v)
		if err := c.ioctl(requestCode(devfs_WRITE, devfs_MAGIC, 1, 1), uintptr(unsafe.Pointer(&m))); err != nil {
			return fmt.Errorf("error setting mode to %v: %v", m, err)
		}
		c.mode = m
	case driver.Bits:
		b := uint8(v)
		if err := c.ioctl(requestCode(devfs_WRITE, devfs_MAGIC, 3, 1), uintptr(unsafe.Pointer(&b))); err != nil {
			return fmt.Errorf("error setting bits per word to %v: %v", b, err)
		}
		c.bits = b
	case driver.Speed:
		s := uint32(v)
		if err := c.ioctl(requestCode(devfs_WRITE, devfs_MAGIC, 4, 4), uintptr(unsafe.Pointer(&s))); err != nil {
			return fmt.Errorf("error setting speed to %v: %v", s, err)
		}
		c.speed = s
	case driver.Order:
		o := uint8(v)
		if err := c.ioctl(requestCode(devfs_WRITE, devfs_MAGIC, 2, 1), uintptr(unsafe.Pointer(&o))); err != nil {
			return fmt.Errorf("error setting bit order to %v: %v", o, err)
		}
	default:
		return fmt.Errorf("unknown key: %v", k)
	}
	return nil
}

func (c *devfsConn) Transfer(tx, rx []byte, delay time.Duration) error {
	p := payload{
		tx:     uint64(uintptr(unsafe.Pointer(&tx[0]))),
		rx:     uint64(uintptr(unsafe.Pointer(&rx[0]))),
		length: uint32(len(tx)),
		speed:  c.speed,
		delay:  uint16(delay.Nanoseconds() / 1000),
		bits:   c.bits,
	}
	// TODO(jbd): Read from the device and fill rx.
	return c.ioctl(msgRequestCode(1), uintptr(unsafe.Pointer(&p)))
}

func (c *devfsConn) Close() error {
	return c.f.Close()
}

// requestCode returns the device specific request code for the specified direction,
// type, number and size to be used in the ioctl call.
func requestCode(dir, typ, nr, size uintptr) uintptr {
	return (dir << devfs_DIRSHIFT) | (typ << devfs_TYPESHIFT) | (nr << devfs_NRSHIFT) | (size << devfs_SIZESHIFT)
}

// msgRequestCode returns the device specific value for the SPI
// message payload to be used in the ioctl call.
// n represents the number of messages.
func msgRequestCode(n uint32) uintptr {
	return uintptr(0x40006B00 + (n * 0x200000))
}

// ioctl makes an IOCTL on the open device file descriptor.
func (c *devfsConn) ioctl(a1, a2 uintptr) error {
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, c.f.Fd(), a1, a2,
	)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}
