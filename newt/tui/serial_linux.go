//go:build linux

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

import "golang.org/x/sys/unix"

const (
	ioctlGetTermios = unix.TCGETS
	ioctlSetTermios = unix.TCSETS
)

var baudRates = map[int]uint32{
	9600: unix.B9600, 19200: unix.B19200, 38400: unix.B38400,
	57600: unix.B57600, 115200: unix.B115200, 230400: unix.B230400,
	460800: unix.B460800, 921600: unix.B921600, 1000000: unix.B1000000,
}

func setSpeed(t *unix.Termios, baud int) bool {
	speed, ok := baudRates[baud]
	if !ok {
		return false
	}
	t.Cflag &^= unix.CBAUD
	t.Cflag |= speed
	t.Ispeed = speed
	t.Ospeed = speed
	return true
}
