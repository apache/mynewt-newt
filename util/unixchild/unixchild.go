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
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

type Config struct {
	SockPath  string
	ChildPath string
	ChildArgs []string
	Depth     int
	MaxMsgSz  int
}

type Client struct {
	FromChild chan []byte
	ToChild   chan []byte
	ErrChild  chan error
	childPath string
	sockPath  string
	childArgs []string
	maxMsgSz  int
	stopping  bool
	stop      chan bool
	stopped   chan bool
}

func New(conf Config) *Client {
	c := &Client{
		childPath: conf.ChildPath,
		sockPath:  conf.SockPath,
		childArgs: conf.ChildArgs,
		maxMsgSz:  conf.MaxMsgSz,
		FromChild: make(chan []byte, conf.Depth),
		ToChild:   make(chan []byte, conf.Depth),
		ErrChild:  make(chan error),
		stop:      make(chan bool),
		stopped:   make(chan bool),
	}

	if c.maxMsgSz == 0 {
		c.maxMsgSz = 1024
	}

	return c
}

func (c *Client) startChild() (*exec.Cmd, error) {
	subProcess := exec.Command(c.childPath, c.childArgs...)

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
			case buf := <-c.ToChild:
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
	if c.stopping {
		return
	}
	c.stopping = true
	log.Debugf("Stopping client")

	c.stop <- true

	select {
	case <-c.stopped:
		log.Debugf("Stopped client")
		return
	}
}

func (c *Client) Start() error {

	l, err := net.Listen("unix", c.sockPath)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd

	go func() {
		for {
			var err error
			cmd, err = c.startChild()
			if err != nil {
				log.Debugf("unixchild start error: %s", err.Error())
				c.ErrChild <- fmt.Errorf("Child start error: %s", err.Error())
			} else {
				fd, err := l.Accept()
				if err != nil {
					log.Debugf("unixchild accept error: %s", err.Error())
				} else {
					c.handleChild(fd)
				}
				cmd.Process.Kill()
				c.ErrChild <- errors.New("Child exited")
			}
			if c.stopping {
				log.Debugf("unixchild exit loop")
				return
			}
			time.Sleep(time.Second)
		}
	}()

	go func() {
		select {
		case <-c.stop:
			l.Close()
			if cmd != nil {
				cmd.Process.Kill()
			}
			os.Remove(c.sockPath)
			c.stopped <- true
		}
	}()

	return nil
}
