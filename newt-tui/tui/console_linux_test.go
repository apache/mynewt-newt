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

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// openPty returns the two ends of a pseudo terminal, which behaves like a
// serial port.
func openPty(t *testing.T) (*os.File, string) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	fd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Skipf("unlockpt: %v", err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Skipf("ptsname: %v", err)
	}
	return master, fmt.Sprintf("/dev/pts/%d", n)
}

func TestSerialConsole(t *testing.T) {
	board, port := openPty(t)
	defer board.Close()

	conn, err := openSerialPort(port, 115200)
	if err != nil {
		t.Fatal(err)
	}
	s := &consoleSession{conn: conn, name: port}
	go s.read()
	defer s.close()

	// The board's output shows up in the console.
	board.Write([]byte("\x1b[32mbtshell>\x1b[0m hello\r\n"))
	for i := 0; i < 50 && !strings.Contains(strings.Join(s.view(5, 0), "\n"),
		"btshell> hello"); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if got := strings.Join(s.view(5, 0), "\n"); !strings.Contains(got,
		"btshell> hello") {
		t.Fatalf("console shows %q", got)
	}

	// A typed command goes to the board with a carriage return.
	if err := s.send("scan"); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 16)
	board.SetReadDeadline(time.Now().Add(time.Second))
	n, _ := board.Read(got)
	if string(got[:n]) != "scan\r" {
		t.Errorf("board got %q", got[:n])
	}

	// Closing the board's side ends the session.
	board.Close()
	for i := 0; i < 50; i++ {
		if closed, _ := s.state(); closed {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("the session should notice that the port closed")
}
