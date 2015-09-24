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

package cli

import (
	curl "github.com/andelf/go-curl"
	"os"
)

type Downloader struct {
}

var Inited bool = false

func NewDownloader() (*Downloader, error) {
	if !Inited {
		curl.GlobalInit(curl.GLOBAL_DEFAULT)
		Inited = true
	}

	dl := &Downloader{}

	return dl, nil
}

func (dl *Downloader) DownloadFile(url string, file string) error {
	easy := curl.EasyInit()
	defer easy.Cleanup()

	easy.Setopt(curl.OPT_URL, url)

	easy.Setopt(curl.OPT_WRITEFUNCTION, func(data []byte, userdata interface{}) bool {
		file := userdata.(*os.File)

		if _, err := file.Write(data); err != nil {
			return false
		}
		return true
	})

	fp, _ := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0755)
	defer fp.Close()

	easy.Setopt(curl.OPT_WRITEDATA, fp)

	if err := easy.Perform(); err != nil {
		return NewNewtError(err.Error())
	}

	return nil
}
