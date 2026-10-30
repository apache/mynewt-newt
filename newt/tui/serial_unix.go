//go:build linux || darwin

/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package tui

import (
	"os"

	"golang.org/x/sys/unix"

	"mynewt.apache.org/newt/util"
)

// openSerial opens a serial port in raw mode at the given speed.
func openSerial(path string, baud int) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, util.FmtNewtError("cannot open %s: %s", path, err)
	}

	t, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		unix.Close(fd)
		return nil, util.FmtNewtError("%s is not a serial port: %s", path, err)
	}

	// The equivalent of cfmakeraw(), 8N1, no flow control.
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON | unix.IXOFF
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.CSTOPB | unix.CRTSCTS
	t.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	if !setSpeed(t, baud) {
		unix.Close(fd)
		return nil, util.FmtNewtError("unsupported baud rate %d", baud)
	}

	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, t); err != nil {
		unix.Close(fd)
		return nil, util.FmtNewtError("cannot configure %s: %s", path, err)
	}
	if err := unix.SetNonblock(fd, false); err != nil {
		unix.Close(fd)
		return nil, util.ChildNewtError(err)
	}
	return os.NewFile(uintptr(fd), path), nil
}
