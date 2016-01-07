/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package protocol

type Echo struct {
	Message string
}

func NewEcho() (*Echo, error) {
	s := &Echo{}
	return s, nil
}

func (e *Echo) EncodeWriteRequest() (*NmgrReq, error) {
	data := []byte(e.Message)

	nmr, err := NewNmgrReq()
	if err != nil {
		return nil, err
	}

	nmr.Op = NMGR_OP_WRITE
	nmr.Flags = 0
	nmr.Group = NMGR_GROUP_ID_DEFAULT
	nmr.Id = 0
	nmr.Len = uint16(len(data))
	nmr.Data = data

	return nmr, nil
}

func DecodeEchoResponse(data []byte) (*Echo, error) {
	e := &Echo{}
	e.Message = string(data[:])

	return e, nil
}
