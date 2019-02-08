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

package unixchild

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type UcAcceptError struct {
	Text string
}

func (err *UcAcceptError) Error() string {
	return err.Text
}

func NewUcAcceptError(text string) *UcAcceptError {
	return &UcAcceptError{
		Text: text,
	}
}

func IsUcAcceptError(err error) bool {
	_, ok := err.(*UcAcceptError)
	return ok
}

type Config struct {
	SockPath      string
	ChildPath     string
	ChildArgs     []string
	Depth         int
	MaxMsgSz      int
	AcceptTimeout time.Duration
}

type clientState uint32

const (
	CLIENT_STATE_STOPPED clientState = iota
	CLIENT_STATE_STARTED
	CLIENT_STATE_STOPPING
)

type Client struct {
	FromChild     chan []byte
	ErrChild      chan error
	toChild       chan []byte
	childPath     string
	sockPath      string
	childArgs     []string
	maxMsgSz      int
	acceptTimeout time.Duration
	stop          chan bool
	stopped       chan bool
	state         clientState
}

func New(conf Config) *Client {
	c := &Client{
		childPath:     conf.ChildPath,
		sockPath:      conf.SockPath,
		childArgs:     conf.ChildArgs,
		maxMsgSz:      conf.MaxMsgSz,
		FromChild:     make(chan []byte, conf.Depth),
		ErrChild:      make(chan error, 1),
		toChild:       make(chan []byte, conf.Depth),
		acceptTimeout: conf.AcceptTimeout,
		stop:          make(chan bool),
		stopped:       make(chan bool),
	}

	if c.maxMsgSz == 0 {
		c.maxMsgSz = 1024
	}

	return c
}

func (c *Client) startChild() (*exec.Cmd, error) {
	subProcess := exec.Command(c.childPath, c.childArgs...)
	subProcess.SysProcAttr = SetSysProcAttrSetPGID()

	stdin, err := subProcess.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdin.Close()

	stdout, _ := subProcess.StdoutPipe()
	stderr, _ := subProcess.StderrPipe()

	if err = subProcess.Start(); err != nil {
		return nil, err
	}

	go func() {
		br := bufio.NewReader(stdout)
		for {
			s, err := br.ReadString('\n')
			if err != nil {
				return
			}
			log.Debugf("child stdout: %s", strings.TrimSuffix(s, "\n"))
		}
	}()

	go func() {
		br := bufio.NewReader(stderr)
		for {
			s, err := br.ReadString('\n')
			if err != nil {
				return
			}
			log.Debugf("child stderr: %s", strings.TrimSuffix(s, "\n"))
		}
	}()

	go subProcess.Wait() // reap dead children

	return subProcess, nil
}

func (c *Client) handleChild(con net.Conn) {
	var wg sync.WaitGroup

	bail := make(chan bool)

	fromDataPump := func() {
		defer wg.Done()
		for {
			var mlen uint16

			err := binary.Read(con, binary.BigEndian, &mlen)
			if err != nil {
				log.Debugln("fromDataPump error: ", err)
				bail <- true
				return
			}

			buf := make([]byte, mlen)
			_, err = io.ReadFull(con, buf)
			if err != nil {
				log.Debugln("fromDataPump error: ", err)
				bail <- true
				return
			}

			c.FromChild <- buf
		}
	}

	toDataPump := func() {
		defer wg.Done()
		for {
			select {
			case buf := <-c.toChild:
				mlen := uint16(len(buf))
				err := binary.Write(con, binary.BigEndian, mlen)
				if err != nil {
					log.Debugln("toDataPump error: ", err)
					return
				}
				_, err = con.Write(buf)
				if err != nil {
					log.Debugln("toDataPump error: ", err)
					return
				}
			case <-bail:
				log.Debugln("toDataPump bail")
				return
			}
		}
	}

	wg.Add(1)
	go fromDataPump()
	wg.Add(1)
	go toDataPump()
	wg.Wait()
}

func (c *Client) Stop() {
	if c.state != CLIENT_STATE_STARTED {
		return
	}

	c.state = CLIENT_STATE_STOPPING
	log.Debugf("Stopping client")

	c.stop <- true

	select {
	case <-c.stopped:
		c.deleteSocket()
		c.state = CLIENT_STATE_STOPPED
		log.Debugf("Stopped client")
		return
	}
}

func (c *Client) acceptDeadline() *time.Time {
	if c.acceptTimeout == 0 {
		return nil
	}

	t := time.Now().Add(c.acceptTimeout)
	return &t
}

func (c *Client) deleteSocket() {
	log.Debugf("deleting socket")
	os.Remove(c.sockPath)
}

func (c *Client) Start() error {
	if c.state != CLIENT_STATE_STOPPED {
		return fmt.Errorf("Attempt to start unixchild twice")
	}

	l, err := net.Listen("unix", c.sockPath)
	if err != nil {
		c.deleteSocket()
		return err
	}

	cmd, err := c.startChild()
	if err != nil {
		err = fmt.Errorf("unixchild start error: %s", err.Error())
		log.Debugf("%s", err.Error())
		c.deleteSocket()
		return err
	}

	if t := c.acceptDeadline(); t != nil {
		l.(*net.UnixListener).SetDeadline(*t)
	}
	fd, err := l.Accept()
	if err != nil {
		err = NewUcAcceptError(fmt.Sprintf("unixchild accept error: %s",
			err.Error()))
		c.deleteSocket()
		return err
	}

	c.state = CLIENT_STATE_STARTED

	go func() {
		c.handleChild(fd)
		c.Stop()
		c.ErrChild <- fmt.Errorf("child process terminated")
	}()

	go func() {
		<-c.stop
		l.Close()
		if cmd != nil {
			cmd.Process.Kill()
		}
		c.deleteSocket()
		c.stopped <- true
	}()

	return nil
}

func (c *Client) TxToChild(data []byte) error {
	if c.state != CLIENT_STATE_STARTED {
		return fmt.Errorf("transmit over unixchild before it is fully started")
	}

	c.toChild <- data
	return nil
}
