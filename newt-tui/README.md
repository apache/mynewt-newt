<!--
#
# Licensed to the Apache Software Foundation (ASF) under one
# or more contributor license agreements.  See the NOTICE file
# distributed with this work for additional information
# regarding copyright ownership.  The ASF licenses this file
# to you under the Apache License, Version 2.0 (the
# "License"); you may not use this file except in compliance
# with the License.  You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
#  KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.
#
-->

# newt-tui

`newt-tui` is a terminal browser for a Mynewt project, in the style of the
[ranger](https://github.com/ranger/ranger) file manager. It shows the
project's targets, apps, BSPs, libraries and unit tests in Vim-like columns
and lets you do the everyday work from one place:

- configure targets: app, BSP, build profile, flash tool and syscfg
  settings, picked from lists with their descriptions at hand
- create, copy, rename and delete targets
- build, run, load, debug and size them, on the board you choose
- flash an mcuboot bootloader and its app in one step
- open a board's serial or RTT console
- see the connected boards, and reset, erase or recover them
- jump from build errors to the line in your editor

A help pane at the bottom of the screen always lists the keys that work
where you are; `?` hides or shows it.

## Starting

`newt-tui` is a separate program: it reads the project with
`newt info --json` and runs newt for everything it does, so newt does not
depend on it. It needs a newt with `newt info --json --brief`. Build it
from this directory and put it in your `PATH`:

```
go build -o ~/bin/newt-tui .
```

Then run it from anywhere inside a newt project:

```
newt-tui
```

`-newt <path>` picks the newt binary it runs (`newt` in `PATH` by
default) and `-C <dir>` opens the project in another directory. Files open
in `$VISUAL`, `$EDITOR` or vi.

## Quick start

1. **Find your target**: `l` opens *Targets*, `j`/`k` move to it. The right
   column shows its app, BSP and build status. `/` filters long lists.
2. **Build it**: `b`. The output appears in a pane at the bottom. If the
   build fails, the list of errors opens; `Enter` on one opens the file at
   that line.
3. **Put it on the board**: `F` flashes the bootloader and the app, `L`
   loads the app; with several boards connected, pick one from the list.
4. **Watch it run**: `C` opens the board's console at the bottom. `Esc`
   goes back to the browser and the console stays open.

## What the browser changes on disk

Only what you ask it to:

- **`target.yml`**: picking an app, BSP, build profile or loader rewrites
  only that `target.<variable>:` line and keeps comments and formatting.
- **The target's `syscfg.yml`**: syscfg values, the flash tool
  (`MYNEWT_DOWNLOADER`) and the default board (`MYNEWT_DEBUGGER_SN`) are
  lines in its `syscfg.vals` map; only those lines change.
- **Target create, copy, rename and delete** run the `newt target`
  commands.
- **`bin/targets/<target>/images/`**: copies of the images the browser
  creates.
- **`~/.newt/tui.yml`**: whether the boards pane is hidden.

## Requirements

Some features use outside tools when they are installed, and explain what
is missing when they are not:

| Tool | Used for |
|---|---|
| `nrfutil` with its `device` command | listing boards and reading their chips; flashing when it is the flash tool |
| SEGGER J-Link software (`JLinkExe`, `JLinkRTTClient`) | flashing with `jlink`, listing boards without nrfutil, the RTT console |
| `nrfjprog` | flashing when it is the flash tool; resetting, erasing and recovering boards without nrfutil |

The serial console works on Linux and macOS.

## For developers

The browser is built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss). `main.go` reads the
flags and finds the project, `newtinfo/` runs `newt info --json` and holds
the types of its output, and everything else is in `tui/`.

The browser learns about the project only from `newt info --json`. It
first reads the project with `--brief`, which is quick, and shows it; then
it reads it again in the background with the targets' builds resolved. A
target's syscfg that is needed before that is read on its own
(`newt info --json <target>`). newt commands run as child processes, and
outside tools are run through package variables such as `runTool` and
`lookPath`, so tests can replace them. Slow work runs in `tea.Cmd`s, so
the screen stays responsive.

### Dependencies and licenses

The browser uses these Go modules; newt itself uses none of them. All of
them are under licenses that the ASF allows to be included (MIT or
BSD-3-Clause, category A). They are downloaded when `newt-tui` is built
and are not part of the source tree.

| Module | License | Used for |
|---|---|---|
| `github.com/charmbracelet/bubbletea` | MIT | the terminal UI framework |
| `github.com/charmbracelet/lipgloss` | MIT | layout and colors |
| `github.com/charmbracelet/x/ansi`, `x/cellbuf`, `x/term`, `colorprofile` | MIT | measuring and truncating text with escape sequences |
| `github.com/muesli/termenv`, `muesli/ansi`, `muesli/cancelreader` | MIT | terminal capabilities, reading input |
| `github.com/lucasb-eyer/go-colorful`, `aymanbagabas/go-osc52/v2`, `xo/terminfo` | MIT | used by Lip Gloss and termenv |
| `github.com/mattn/go-isatty`, `mattn/go-runewidth`, `rivo/uniseg` | MIT | terminal detection, character widths |
| `github.com/mattn/go-localereader`, `erikgeiser/coninput` | MIT | reading input on Windows (go-localereader states its license in its README) |
| `golang.org/x/sys`, `golang.org/x/text` | BSD-3-Clause | serial ports (x/sys); text handling in Bubble Tea |
| `github.com/kballard/go-shellquote` | MIT | quoting the commands shown in the output pane; newt uses it too |

Run the tests with `go test ./...`. They drive the model with key presses
and check what it does and shows, with fake projects, boards and tools.
