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

package porcelain

func testInfo() *Info {
	return &Info{
		FormatVersion: FormatVersion,
		NewtVersion:   "1.16.0",
		Project:       Project{Name: "proj", Path: "/home/user/proj"},
		Repos: []Repo{
			{
				Name: "apache-mynewt-core",
				Path: "repos/apache-mynewt-core",
				Origin: RepoOrigin{
					Type: "github",
					Url:  "https://github.com/apache/mynewt-core.git",
					User: "apache",
					Repo: "mynewt-core",
				},
				Requested: "1.12.0",
				Installed: "1.12.0",
				Commit:    "c81875e2",
			},
			{Name: "proj", Path: ".", Local: true},
		},
		Targets: []Target{
			{
				Name:     "targets/blinky",
				Repo:     "proj",
				Path:     "targets/blinky",
				Resolved: true,
				Vars: map[string]string{
					"bsp":    "@apache-mynewt-core/hw/bsp/nordic_pca10056",
					"app":    "@apache-mynewt-core/apps/blinky",
					"cflags": "-DFOO -DBAR",
				},
				BuildProfiles: []string{"debug", "default"},
				Artifacts: Artifacts{
					BinDir: "bin/targets/blinky",
					Elf:    "bin/targets/blinky/app/apps/blinky/blinky.elf",
					Img:    "bin/targets/blinky/app/apps/blinky/blinky.img",
				},
				FlashArea: "FLASH_AREA_IMAGE_0",
				Syscfg: []Setting{
					{
						Name:        "CONSOLE_UART_BAUD",
						Value:       "1000000",
						Default:     "115200",
						Description: "Console UART baud rate.",
						Type:        "raw",
						State:       "good",
						Package:     "@apache-mynewt-core/sys/console",
						Choices:     []string{},
						Restrictions: []Restriction{
							{Type: "notnull"},
							{Type: "expr", Expr: "CONSOLE_UART"},
						},
						History: []SettingPoint{
							{"@apache-mynewt-core/sys/console", "115200"},
							{"targets/blinky", "1000000"},
						},
					},
					{
						Name:         "LOG_LEVEL",
						Value:        "",
						Package:      "newt",
						Choices:      []string{"0", "1"},
						Restrictions: []Restriction{},
						History:      []SettingPoint{},
					},
				},
				SyscfgErrors:   []string{"Setting history (newest -> oldest):"},
				SyscfgWarnings: []string{"Ignoring override of undefined settings:"},
				Deps: []Dep{
					{
						Name:       "@apache-mynewt-core/apps/blinky",
						RequiredBy: []DepReason{},
					},
					{
						Name: "@apache-mynewt-core/sys/console/full",
						RequiredBy: []DepReason{
							{
								Name: "@apache-mynewt-core/kernel/os",
								Apis: []string{"console"},
							},
							{
								Name:   "@apache-mynewt-core/sys/console",
								Syscfg: "CONSOLE_IMPLEMENTATION == full",
								Apis:   []string{},
							},
						},
					},
				},
			},
		},
		Packages: []Package{
			{
				Name:        "@apache-mynewt-core/apps/blinky",
				Type:        "app",
				Repo:        "apache-mynewt-core",
				Path:        "repos/apache-mynewt-core/apps/blinky",
				Description: "Blinks a LED.\nTwice \\ a second.",
				Keywords:    []string{"blink", "led"},
				Deps:        []string{"@apache-mynewt-core/kernel/os"},
				Apis:        []string{},
				ReqApis:     []string{"console"},
			},
			{
				Name:       "@apache-mynewt-core/hw/bsp/nordic_pca10056",
				Type:       "bsp",
				Repo:       "apache-mynewt-core",
				Path:       "repos/apache-mynewt-core/hw/bsp/nordic_pca10056",
				Author:     "Apache Mynewt <dev@mynewt.apache.org>",
				Keywords:   []string{},
				Deps:       []string{},
				Apis:       []string{},
				ReqApis:    []string{},
				SyscfgDefs: 2,
				Bsp: &Bsp{
					Arch:     "cortex_m4",
					Compiler: "compiler/arm-none-eabi-m4",
					FlashMap: []FlashArea{
						{"FLASH_AREA_BOOTLOADER", 0, 0, 0, 32768},
						{"FLASH_AREA_IMAGE_0", 1, 0, 0xc000, 483328},
					},
				},
			},
		},
	}
}
