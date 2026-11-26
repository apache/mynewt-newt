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

import (
	"bytes"
	"testing"
)

func TestWritePorcelain(t *testing.T) {
	var buf bytes.Buffer
	if err := WritePorcelain(&buf, testInfo()); err != nil {
		t.Fatal(err)
	}

	want := `project proj format_version 1
project proj newt_version 1.16.0
project proj path /home/user/proj
repo apache-mynewt-core path repos/apache-mynewt-core
repo apache-mynewt-core local false
repo apache-mynewt-core origin.type github
repo apache-mynewt-core origin.url https://github.com/apache/mynewt-core.git
repo apache-mynewt-core origin.user apache
repo apache-mynewt-core origin.repo mynewt-core
repo apache-mynewt-core requested 1.12.0
repo apache-mynewt-core installed 1.12.0
repo apache-mynewt-core commit c81875e2
repo apache-mynewt-core needs_upgrade false
repo proj path .
repo proj local true
repo proj needs_upgrade false
target targets/blinky repo proj
target targets/blinky path targets/blinky
target targets/blinky var.app @apache-mynewt-core/apps/blinky
target targets/blinky var.bsp @apache-mynewt-core/hw/bsp/nordic_pca10056
target targets/blinky var.cflags -DFOO -DBAR
target targets/blinky build_profiles debug default
target targets/blinky artifact.bin_dir bin/targets/blinky
target targets/blinky artifact.elf bin/targets/blinky/app/apps/blinky/blinky.elf
target targets/blinky artifact.img bin/targets/blinky/app/apps/blinky/blinky.img
target targets/blinky resolved true
target targets/blinky boot_loader false
target targets/blinky flash_area FLASH_AREA_IMAGE_0
target targets/blinky syscfg_error Setting history (newest -> oldest):
target targets/blinky syscfg_warning Ignoring override of undefined settings:
target targets/blinky syscfg.CONSOLE_UART_BAUD.value 1000000
target targets/blinky syscfg.CONSOLE_UART_BAUD.default 115200
target targets/blinky syscfg.CONSOLE_UART_BAUD.description Console UART baud rate.
target targets/blinky syscfg.CONSOLE_UART_BAUD.type raw
target targets/blinky syscfg.CONSOLE_UART_BAUD.state good
target targets/blinky syscfg.CONSOLE_UART_BAUD.package @apache-mynewt-core/sys/console
target targets/blinky syscfg.CONSOLE_UART_BAUD.restriction notnull
target targets/blinky syscfg.CONSOLE_UART_BAUD.restriction expr CONSOLE_UART
target targets/blinky syscfg.CONSOLE_UART_BAUD.history @apache-mynewt-core/sys/console 115200
target targets/blinky syscfg.CONSOLE_UART_BAUD.history targets/blinky 1000000
target targets/blinky syscfg.LOG_LEVEL.package newt
target targets/blinky syscfg.LOG_LEVEL.choices 0 1
target targets/blinky dep @apache-mynewt-core/apps/blinky
target targets/blinky dep @apache-mynewt-core/sys/console/full
target targets/blinky dep.@apache-mynewt-core/sys/console/full.required_by @apache-mynewt-core/kernel/os api:console
target targets/blinky dep.@apache-mynewt-core/sys/console/full.required_by @apache-mynewt-core/sys/console CONSOLE_IMPLEMENTATION == full
package @apache-mynewt-core/apps/blinky type app
package @apache-mynewt-core/apps/blinky repo apache-mynewt-core
package @apache-mynewt-core/apps/blinky path repos/apache-mynewt-core/apps/blinky
package @apache-mynewt-core/apps/blinky description Blinks a LED.\nTwice \\ a second.
package @apache-mynewt-core/apps/blinky keywords blink led
package @apache-mynewt-core/apps/blinky dep @apache-mynewt-core/kernel/os
package @apache-mynewt-core/apps/blinky req_api console
package @apache-mynewt-core/hw/bsp/nordic_pca10056 type bsp
package @apache-mynewt-core/hw/bsp/nordic_pca10056 repo apache-mynewt-core
package @apache-mynewt-core/hw/bsp/nordic_pca10056 path repos/apache-mynewt-core/hw/bsp/nordic_pca10056
package @apache-mynewt-core/hw/bsp/nordic_pca10056 author Apache Mynewt <dev@mynewt.apache.org>
package @apache-mynewt-core/hw/bsp/nordic_pca10056 syscfg_defs 2
package @apache-mynewt-core/hw/bsp/nordic_pca10056 bsp.arch cortex_m4
package @apache-mynewt-core/hw/bsp/nordic_pca10056 bsp.compiler compiler/arm-none-eabi-m4
package @apache-mynewt-core/hw/bsp/nordic_pca10056 bsp.flash_area.FLASH_AREA_BOOTLOADER id=0 device=0 offset=0x0 size=32768
package @apache-mynewt-core/hw/bsp/nordic_pca10056 bsp.flash_area.FLASH_AREA_IMAGE_0 id=1 device=0 offset=0xc000 size=483328
`
	if got := buf.String(); got != want {
		t.Errorf("unexpected output:\n%s\nwant:\n%s", got, want)
	}
}
