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

# newt tui

`newt tui` is a terminal browser for a Mynewt project, in the style of the
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
- see the project's repos, their branches and history, and switch a repo
  to another branch

You do not need this document to get around: the help pane at the bottom of
the screen always lists the keys that work where you are. The document is
also built into the browser: `g?` opens it at the section about where you
are, `]`/`[` go to the next/previous section, `/` searches and `Esc` closes
it.

- [Starting](#starting)
- [Quick start: your first session](#quick-start-your-first-session)
- [How do I…](#how-do-i)
- [The screen](#the-screen)
- [Moving around](#moving-around)
- [Targets](#targets)
- [Target settings](#target-settings)
- [Syscfg settings](#syscfg-settings)
- [Running newt commands](#running-newt-commands)
- [Choosing a board](#choosing-a-board)
- [The boards pane](#the-boards-pane)
- [Bootloader and images](#bootloader-and-images)
- [The console](#the-console)
- [Build errors](#build-errors)
- [Size and dependencies](#size-and-dependencies)
- [Repos](#repos)
- [The editor](#the-editor)
- [What the browser changes on disk](#what-the-browser-changes-on-disk)
- [Requirements](#requirements)
- [Troubleshooting](#troubleshooting)
- [Key reference](#key-reference)
- [For developers](#for-developers)

## Starting

Run it from anywhere inside a newt project:

```
newt tui
```

`newt --tui` and `newt -t` do the same. On the first launch the browser
asks which editor should open files; see [The editor](#the-editor).

## Quick start: your first session

From building to watching your app run on a board:

1. **Open it**: `newt tui` in your project. Answer the editor question with
   `v` (vim) or `n` (nano).
2. **Find your target**: `l` opens *Targets*, `j`/`k` move to it. The right
   column shows its app, BSP and build status. `/` and part of a name
   filters long lists.
3. **Build it**: `b`. The output appears in a pane at the bottom. If the
   build fails, the list of errors opens; `Enter` on one opens the file at
   that line.
4. **Put it on the board**:
   - an app with a bootloader (e.g. btshell with mcuboot) on a board that
     does not have the bootloader yet: `F`, then `Enter` on your board;
   - afterwards: `L`, *create a new image…*, `Enter` to accept the
     suggested version, and `Enter` on your board.
5. **Watch it run**: `C`, `Enter` on your board. The board's console opens
   at the bottom; type a command (e.g. `help` in btshell) and `Enter`.
   `Esc` goes back to the browser and the console stays open.

## How do I…

**…make a target for my board?** In *Targets*, `n`, type a name (e.g.
`my_btshell`) and `Enter`. Its settings open: on *app*, `l`, type
`/btshell`, `Enter` to keep the filter and `Enter` to choose; on *bsp*,
`j` `l`, `/pca10056`, `Enter`, `Enter`. For an app that runs under mcuboot,
make a bootloader target the same way, with the app `@mcuboot/boot/mynewt`,
the same BSP and the profile `optimized`.

**…flash a new board from scratch?** `F` on the app's target: it builds and
loads the bootloader, then builds, creates the image and loads the app.

**…load a new version after changing code?** `L`, then *create a new
image…*. The next version is suggested (1.0.3 after 1.0.2); `Enter`, pick
the board, `Enter`. It builds, creates the image and loads it.

**…go back to an older version?** `L` lists the images the browser saved,
newest first; pick one and the board.

**…see what the board prints after a load?** Open the console first (`C`),
`Esc` back to the browser, then load (`L`): the load's output and the
console are shown side by side, and the board's restart shows up in the
console.

**…debug?** Load the build first (`L` or `F`), as `newt debug` attaches to
whatever runs on the board. Then `d` and pick the board. In gdb,
`layout src` shows the source; `quit`, then `Enter`, returns to the
browser. `r` builds, loads and debugs in one go.

**…use a particular board when several are connected?** In the board list,
`s` instead of `Enter` runs on the board and makes it the target's default,
so the list starts on it next time.

**…change a setting such as `BLE_MAX_CONNECTIONS`?** `l` on the target, `G`
to *syscfg*, `l`. Type `/max_conn`, `Enter`; `Enter` again on the setting,
type the value, `Enter`.

**…flash with J-Link instead of nrfutil?** `l` on the target, *flash*, `l`,
pick *jlink*. *bsp default* goes back to the BSP's choice.

**…find out what uses my flash, and why a package is there?** `s` on a
built target lists the packages by size; the preview of each says which
packages pull it in.

**…stop a build?** `Ctrl-C`. It never quits the browser; `q` does.

**…edit a target by hand?** `e` opens its `target.yml` in your editor; the
browser reloads when you close it.

## The screen

```
 newt nimbleproj › Targets › nrf52_btshell                          title bar
 Targets      5 │ bttester         │ Target    targets/nrf52_btshell
 Apps        73 │ my_blinky_sim    │ App       @apache-mynewt-nimble/apps/btshell
 BSPs        92 │ nrf52_boot       │ BSP       @apache-mynewt-core/hw/bsp/nordic_pca10056
 Libraries  414 │ nrf52_btshell    │ Profile   debug
 Unit tests  44 │ ycm_btshell      │ Status    built 2026-10-22 18:01:11, 309.0 KiB
                │                  │ Boot      nrf52_boot  (gB to go there, F flashes both)
                │                  │ Image     1.0.0.0
 ───────────────────────────────────────────────────────────────────────────
 Move                   Run on nrf52_btshell   Go to                          help pane
 j/k  down / up         b  build               ga  app apps/btshell
 h    back              L  load                gb  bsp hw/bsp/nordic_pca10056
 ...
 . show other repos too · R reload project · V editor: vim · ? hide this help · q quit
 status line                                                     local 4/5
```

- **The columns**: the left column is the level you came from, the middle
  column is the current list, and the right column previews the selected
  item: a target's configuration, a package's description and
  dependencies, the choices for a setting, and so on.
- **The title bar** shows the project and where you are in it, e.g.
  `newt nimbleproj › Targets › nrf52_btshell › syscfg › BLE_MAX_CONNECTIONS`.
  The project is named after the directory that holds `project.yml`. If
  `project.yml` gives it a different name, that name follows dimmed in
  parentheses (the `my_project` placeholder of `newt new` is left out).
- **The git pane** shows in the left column, in two parts. *This
  package*: the repo of a package, the branch it has checked out, and the
  latest commits that changed the package's directory. *Whole repo*: the
  latest commits of the package's repo, whatever they changed. The package
  gets up to half of the lines, the repo the rest; on a short terminal only
  the package's part is shown. While you browse apps, BSPs, libraries and
  unit tests it shows the selected package, under the categories; in a
  target's settings it shows the app or BSP under the cursor, under the
  targets, and while you choose a target's app or BSP, the one under the
  cursor. On a package, `c` lists all of its commits. `zg` hides or shows
  the pane.

  ```
   Repos                       3 │ apps/bleuart
  ── git: this package ──────────│ apps/blues-wireless
   apache-mynewt-core · master   │ apps/bsncent
   0adba9d68 21-12-01 baselibc: …│ apps/bsnprph
   10ad0ca74 19-10-07 apps/bleua…│ apps/bus_test
  ── git: whole repo ────────────│ apps/buzzer
   6710eb6b0 26-10-09 tools: Add…│ apps/coremark
   c17a74e96 26-10-07 ci: Add in…│ apps/crypto_test
  ```
- **The help pane** lists the keys for the current position, grouped by
  what they do, and changes as you move. `?` hides or shows it. On a narrow
  terminal it uses one line per group, and under 16 rows it hides itself.
- **The status line** shows messages and errors, prompts, the active
  filter and the cursor position. `local` means entries from other repos
  are hidden (see `.` below).

## Moving around

| Key | Does |
|---|---|
| `j` / `k`, arrows | down / up |
| `h` / `l`, arrows, `Enter` | back / open |
| `gg` / `G`, `Home` / `End` | first / last |
| `Ctrl-d` / `Ctrl-u` | half a page down / up |
| `Ctrl-f` / `Ctrl-b` | a page down / up |
| `]` / `[` | next / previous repo in a list of targets or packages |
| `/` | filter the list: type, then `Enter` to keep the filter or `Esc` to clear it |
| `.` or `zh` | show or hide entries from repos other than the project's own |
| `ga` `gb` `gl` `gB` | from a target, go to its app, BSP, loader or bootloader target |
| `zg` | hide or show the git pane |
| `H` | go back to where you were before a `g` jump |
| `R` | reload the project from disk, e.g. after `newt upgrade` |
| `?` | hide or show the help pane |
| `q` | quit (asks first while a command runs) |

The categories are **Targets**, **Apps**, **BSPs**, **Libraries**,
**Unit tests** and **Repos**. Names are shown without their repo
(`hw/bsp/nordic_pca10056`). When a list has entries from more than one
repo, they are grouped under a header per repo, your own first, so that
e.g. the apps of apache-mynewt-core and of NimBLE are kept apart:

```
 Apps         71 │ ── my_project ──────────────────────
 BSPs         92 │ apps/blinky
 Libraries   398 │ ── apache-mynewt-core ──────────────
 Unit tests   43 │ apps/bleprph_oic
                 │ apps/blesplit
                 │ ...
                 │ ── apache-mynewt-nimble ────────────
                 │ apps/advertiser
                 │ apps/blecent
```

With a single repo in the list (e.g. after filtering) there are no headers,
and the repo is shown dimmed next to each name instead.

Like `newt target show`, the Targets list shows only the project's own
targets at first; `.` also shows the example targets that come with
apache-mynewt-core, NimBLE and so on. The other categories show all repos.

On a package, `l` or `e` opens its `pkg.yml` in your editor, and `c`
lists the commits that changed it, with each commit's message and changed
files in the preview. On a target,
`l` opens its settings and `e` opens `target.yml`. The `g` jumps are only
offered for what the target has: `gl` when it uses a loader, `gB` when it
has a bootloader target. The preview of an app or BSP lists the targets
that use it.

Lists remember where you were when you go back and forth, and clearing a
filter keeps the cursor on the selected entry.

## Targets

In the Targets list:

| Key | Does |
|---|---|
| `n` | create a target: type a name, then its settings open so that you can pick the app and BSP |
| `y` | copy the target (`<target>_copy` is suggested) |
| `a` | rename the target |
| `D` | delete the target and its directory, after a `y/N` question |

Names work as on the command line: `foo` means `targets/foo`. These keys
run `newt target create`, `copy` and `delete`; newt has no rename command,
so a rename is a copy followed by a delete. Errors from newt, such as
`Target already exists`, appear in the status line.

Targets from other repos are read-only: you can browse their settings, but
not change, rename or delete them.

## Target settings

`l` on a target opens its settings:

```
 app      apps/btshell              @apache-mynewt-nimble
 bsp      hw/bsp/nordic_pca10056
 profile  debug
 flash    nrfutil                   8.2.1
 syscfg   2 overrides
```

`l` on a setting lists the choices, with the current one marked
`current`. Move to a choice, read its preview, and press `Enter` to save
it; `h` goes back without changing anything. `/` filters the choices.

- **app**: all apps of the project and its repos.
- **bsp**: all BSPs.
- **profile**: the build profiles that the BSP's compiler defines
  (`compiler.flags.<profile>` in its `compiler.yml`), typically `debug`,
  `default`, `optimized` and `speed`.
- **flash**: the tool `newt load` uses: `nrfutil`, `nrfjprog` or `jlink`,
  each with its installed version, or *not installed*. The BSP chooses a
  default; picking a tool here overrides it for this target (the
  `MYNEWT_DOWNLOADER` syscfg setting), and *bsp default* removes the
  override. The preview shows the program the tool runs and the syscfg
  settings it uses, e.g. `JLINK_TARGET` for J-Link.
- **loader**: only shown for targets that already use one. A loader is
  part of Mynewt's rarely used *split image* mode; it is not the
  bootloader. *none* removes it.
- **syscfg**: see the next section.

## Syscfg settings

`l` on **syscfg** shows every setting of the target's build, resolved the
way `newt target config show` resolves them:

```
 ● BLE_MAX_CONNECTIONS          4 │ BLE_MAX_CONNECTIONS
   BLE_ATT_PREFERRED_MTU      256 │ Value     4
   CONSOLE_HISTORY           none │ Set by    this target
                                  │ Default   1
                                  │ Defined   nimble
                                  │ Group     Bluetooth › Connections
                                  │
                                  │ The maximum number of concurrent connections.
                                  │
                                  │ History, newest first
                                  │ 4            target
                                  │ 1            default
```

Settings this target sets are marked `●` and listed first; values changed
by the app, the BSP or a package are in normal text; untouched defaults are
grey. The preview shows who set the value, the default, the defining
package, the type, choices and rules (`requires X > 0`), the description
and the history of overrides.

| Key | Does |
|---|---|
| `/` | search setting names *and* descriptions, e.g. `/connections` |
| `Enter` | change the value: settings with fixed choices open a list, others a prompt with the current value filled in |
| `space` | toggle a 0/1 setting |
| `x` | remove this target's value, so the app, BSP or package value applies again |
| `o` | show only settings that differ from their default |
| `B` | show only the Bluetooth essentials (for builds with NimBLE) |
| `h` | back to the target's settings |

Values are saved in the target's `syscfg.yml`, which overrides the app, the
BSP and every package. After each change the configuration is resolved
again, so settings that depend on the new value appear or disappear right
away, and a value that breaks a rule is reported straight away (e.g.
`Setting BLE_L2CAP_COC_SDU_BUFF_COUNT(0) requires: BLE_L2CAP_COC_SDU_BUFF_COUNT > 0`)
instead of at the next build.

Constant settings cannot be changed, and the settings of targets from
other repos are read-only.

### The Bluetooth page

For builds that include NimBLE, `B` limits the list to the settings that
matter most day to day:

| Group | Settings |
|---|---|
| Roles | `BLE_ROLE_CENTRAL`, `BLE_ROLE_PERIPHERAL`, `BLE_ROLE_BROADCASTER`, `BLE_ROLE_OBSERVER` |
| Connections | `BLE_MAX_CONNECTIONS`, `BLE_ATT_PREFERRED_MTU`, `BLE_GATT_MAX_PROCS`, `BLE_L2CAP_COC_MAX_NUM`, `BLE_LL_MAX_PKT_SIZE`, `BLE_LL_CONN_INIT_MAX_TX_BYTES` |
| Advertising | `BLE_EXT_ADV`, `BLE_EXT_ADV_MAX_SIZE`, `BLE_MULTI_ADV_INSTANCES`, `BLE_PERIODIC_ADV` |
| Version and PHY | `BLE_VERSION`, `BLE_PHY_2M`, `BLE_PHY_CODED`, `BLE_LL_CFG_FEAT_LE_2M_PHY`, `BLE_LL_CFG_FEAT_LE_CODED_PHY`, `BLE_LL_TX_PWR_DBM` |
| Security | `BLE_SM_LEGACY`, `BLE_SM_SC`, `BLE_SM_BONDING`, `BLE_SM_MITM`, `BLE_SM_IO_CAP`, `BLE_STORE_CONFIG_PERSIST`, `BLE_STORE_MAX_BONDS` |
| Identity | `BLE_SVC_GAP_DEVICE_NAME` |
| Buffers | `BLE_TRANSPORT_ACL_COUNT`, `BLE_TRANSPORT_EVT_COUNT` |
| Logging | `BLE_HS_LOG_LVL`, `BLE_HS_DEBUG` |

Settings that the build does not have are left out. All the usual keys
work on the page; `B` again shows all settings.

## Running newt commands

On a target:

| Key | Runs |
|---|---|
| `b` | `newt build` |
| `r` | `newt run` (build, create the image, load and debug) |
| `L` | `newt load` on a board you choose; for apps with a bootloader, of an image you choose |
| `d` | `newt debug` on a board you choose |
| `C` | the board's console |
| `s` | the size view |
| `c` | `newt clean`, after a `y/N` question |
| `i` | `newt create-image`, asking for the version (apps with a bootloader) |
| `F` | flash bootloader + app (apps with a bootloader target) |
| `E` | the errors and warnings of the target's last build |

On a unit test package, `t` runs `newt test`.

Build, load, clean, test and *flash bootloader + app* run in the
background, with their output in a pane at the bottom; the browser stays
usable meanwhile:

```
── newt build targets/nrf52_btshell · 12s · running · Ctrl-C stops it ──────
 Compiling repos/apache-mynewt-nimble/nimble/host/src/ble_gap.c
 Compiling repos/apache-mynewt-nimble/nimble/host/src/ble_gattc.c
```

- One command runs at a time. `Ctrl-C` stops it, including the compilers
  it started; it never quits the browser.
- When it is done, the pane's title says *done*, *failed* or *stopped* and
  the status line says how long it took. `Esc` hides the pane and `O`
  shows it again.
- The project is reloaded afterwards, so new images and build states show
  up right away.

`debug` and `run` get the terminal to themselves, so gdb works as on the
command line, including its TUI mode (`layout src`). When gdb exits, the
browser waits for `Enter` and comes back.

The commands run the same `newt` binary as the browser, with your normal
environment.

## Choosing a board

`L`, `d`, `C` and `F` first list the boards connected to the computer:

```
 newt nimbleproj › Targets › nrf52_btshell › board for load
   PCA10056 · J-Link · NRF52840   000683220560 │ Serial    000683220560
 ⚠ PCA10095 · J-Link · NRF5340    001050072337 │ Board     PCA10056
   let the tool pick                           │ Ports     /dev/ttyACM0
                                               │ Runs
                                               │ JLINK_SN=000683220560 newt load targets/nrf52_btshell
                                               │ Chip
                                               │ Part      NRF52840 xxAA, revision 2
                                               │ Access    not protected
                                               │ Target    built for nRF52840 (MCU_TARGET)
                                               │ ✓ matches this board
```

| Key | Does |
|---|---|
| `Enter` | run on the selected board, this time only |
| `s` | run on it and make it this target's default board |
| `r` | scan for boards again |
| `p` | for the console: use the board's next serial port |
| `h` / `Esc` | cancel |

The boards come from `nrfutil device list`, or from J-Link Commander's
`ShowEmuList` if nrfutil is not installed. The preview shows the exact
command that will run. *Let the tool pick* runs without choosing a board,
as plain `newt load` does.

**The chip check.** After the scan, the browser reads the chip on each
board in the background with `nrfutil device device-info`, which only reads
the chip's ID registers. The preview shows the part, revision and whether
the chip is readback-protected, and compares the part with the chip the
target is built for (`MCU_TARGET`). A board with a different chip is marked
`⚠`, and running on it asks *anyway? (y/N)* first. For a protected chip
the preview shows the command that unlocks it (`nrfutil device recover`,
which erases the chip). Loading never runs it for you; the
[boards pane](#the-boards-pane) does, after asking.

**How the board is picked.** All flash and debug scripts of
apache-mynewt-core pick the board from the `JLINK_SN` environment variable,
so a one-off choice does not change any file. `s` stores the serial number
in the target's `MYNEWT_DEBUGGER_SN` syscfg setting, which the scripts fall
back to.

Before loading, `L` checks that the target's flash tool is installed, and
if not, says which installed tool to pick under the target's settings ›
flash.

`newt debug` attaches to the firmware already on the board; it does not
load the build. If gdb shows `?? ()` instead of function names, load the
build first, or use `r`. The board list reminds you of this for `d`.

## The boards pane

A pane on the right lists the boards connected to the computer. It is open
from the start; `zb` moves the cursor into it. It looks for boards every 5
seconds, so boards you plug in or out show up without a key press, and
reads the chip of each new board once, as the board list does. Under the list are the
details of the selected board: serial number, probe, chip, flash size,
readback protection and serial ports.

```
 nrf52_btshell     │── boards ────────────────────────
 ycm_btshell       │ PCA10056 · J-Link · NRF52840
                   │ ⚠ PCA10095 · J-Link · NRF5340
                   │
                   │ Serial    000683220560
                   │ Probe     J-Link
                   │ Chip      NRF52840
                   │ Access    not protected
                   │ Ports     /dev/ttyACM0
```

| Key | Does |
|---|---|
| `j` / `k` | move between the boards |
| `r` | reset the board |
| `E` | erase all of its flash, after asking |
| `X` | recover it, after asking: erase the whole chip and remove its readback protection |
| `s` | look for boards now |
| `h` / `Esc` | back to the lists; the pane stays open |
| `zb` | hide the pane (from the lists: move into it again) |

A hidden pane stays hidden the next time you start the browser, until `zb`
shows it again; the choice is saved in `~/.newt/tui.yml`.

The actions run `nrfutil device reset`, `erase --all` or `recover` with the
board's serial number, or `nrfjprog --reset`, `--eraseall` or `--recover`
when nrfutil is not installed. A board with a protected chip is marked
`⚠`; after an erase or a recover its chip is read again.

**Chips with two cores.** On an nRF53, erase and recover work on one core
at a time, so `E` and `X` ask which: `a` the application core, `n` the
network core (e.g. the Bluetooth controller), or `b` both, the network
core first. Picking a core answers the question; `Esc` cancels. They run
with `--core application` / `--core network`, or nrfjprog's
`--coprocessor CP_APPLICATION` / `CP_NETWORK`. The board's details list
its cores. The question needs the chip to have been read; on a board found
only by J-Link, erase and recover use the tool's default, the application
core. Reset does not ask: it resets through the application core, the
tool's default.

The pane leaves the probes alone while a newt command runs, so it never
gets in the way of a load or a debug session, and no chips are read while
the RTT console is open. It needs a terminal at least 100 columns wide.

## Bootloader and images

Apps on BSPs whose flash map has a bootloader area run under a bootloader
such as mcuboot. The bootloader is a target of its own (app
`@mcuboot/boot/mynewt` and the same BSP), loaded once; the app is loaded as
an image (`.img`) made by `newt create-image`.

For such an app, the browser looks for a bootloader target with the same
BSP, preferring the project's own over ones from repos. When it finds one,
the target's preview shows it together with the current image:

```
 Boot      nrf52_boot  (gB to go there, F flashes both)
 Image     1.0.0.0
```

and `gB` goes to the bootloader target and `F` flashes both. Without one,
the preview says so and explains how to create it, and `gB` and `F` are not
offered. A bootloader target lists the targets it boots.

| Key | Does |
|---|---|
| `L` | load: choose one of the app's images, or create a new one |
| `i` | create the image: asks for the version (the current one, or the next free one, is filled in) |
| `F` | flash bootloader + app on a board you choose: builds and loads the bootloader, then builds the app, creates its image and loads it |
| `gB` | go to the bootloader target |

### Choosing the image to load

For these apps, `L` first lists the images that are ready, newest first:

```
 newt nimbleproj › nrf52_btshell › load nrf52_btshell: choose the image
 1.0.2.0                  latest · 2026-10-23 11:53 │ Image 1.0.2.0
 1.0.1.0                         2026-10-23 10:20 │ Made      2026-10-23 11:53:10
 1.0.0.0                         2026-10-22 18:02 │ Build     from the current build
 create a new image…
```

- **An image**: `Enter` picks the board and loads it
  (`newt load --imgfile`). Images made before the app's last build are grey
  and marked *from an older build*; you can still load them, e.g. to go
  back to an earlier version.
- **create a new image…**: asks for the version, with the next one
  suggested (1.0.3 after 1.0.2), picks the board, and then builds the app,
  creates the image and loads it.
- `x` deletes a saved image, after asking.

`newt build` deletes the image that `newt create-image` writes next to the
build. So that earlier images stay available, the browser keeps a copy of
every image it creates in `bin/targets/<target>/images/`, e.g.
`btshell-1.0.2.0.img`. `newt clean` deletes these too; `c` says so when
there are saved images.

## The console

`C` opens the board's console (e.g. btshell's shell) in a pane at the
bottom of the browser, over the transport the target's syscfg enables:

```
── console /dev/ttyACM0 at 115200 baud · Esc: back to the browser · Ctrl-] closes ──
 btshell> GAP procedure initiated: stop advertising.
 000002 Host and controller synced
 > help█
```

- **UART** (`CONSOLE_UART: 1`): the browser opens the board's serial port
  itself at the target's `CONSOLE_UART_BAUD`. The port is the board's own
  from the board list; `p` there switches between boards with several
  ports (the nRF5340 DK has two). Without a board in the list, the only
  USB serial port on the computer is used. Linux and macOS only.
- **RTT** (`CONSOLE_RTT: 1`): the browser keeps a J-Link Commander
  connection to the chip open (device `JLINK_TARGET`) and reads RTT from
  `localhost:19021`. While it is open, loading over J-Link may fail, as the
  probe is in use.

| Key | Does |
|---|---|
| `Enter` | send the line to the board |
| `↑` / `↓` | earlier commands |
| `PgUp` / `PgDn` | scroll the output |
| `Esc` | back to the browser; the console stays open and keeps receiving |
| `Ctrl-]` | close the console |

With the console open, `C` types in it again and `Ctrl-]` closes it from
anywhere. A command that runs meanwhile, e.g. a load, shows its output
next to the console, so you can watch the board restart.

## Build errors

When `b` fails, the browser reads the compiler, linker and newt messages
from the output and lists them, errors first:

```
 newt nimbleproj › my_blinky_sim › build of my_blinky_sim: 2 errors, 1 warning
 ✗ main.c:45  'foo' undeclared (first use in this function)  │ Error
 ✗ main.c:44  unused variable 'x' [-Werror=unused-variable]  │ 'foo' undeclared (first use in this function)
 ! Following external repos are modified or missing, ...     │ File      apps/blinky/src/main.c:45:5
                                                             │    44       int x;
                                                             │    45 ▶     foo = 1;
                                                             │    46       int rc;
```

The preview shows the full message, the compiler's notes and the source
around the line. `Enter` opens the file in your editor at that line, `/`
filters the list and `h` closes it. `E` on the target shows the last
build's list again.

A build that succeeds with warnings says so in the status line
(`done, 3 warnings (E to see them)`). A newt error without a source
location, such as a missing source file, is shown in the status line.

## Size and dependencies

`s` on a built target runs `newt size` and shows the flash or RAM each
package takes, largest first:

```
 newt nimbleproj › nrf52_boot › size of nrf52_boot by flash
 Total  13.2 KiB flash of 32.0 KiB (41%), 7.1 KiB RAM of 256.0 KiB (3%)
 boot/bootutil                    5.7 KiB █████████
 hw/mcu/nordic/nrf52xxx           2.7 KiB ████
 crypto/mbedtls                   1.0 KiB █
 ...
```

The totals are measured against the image's flash area
(`FLASH_AREA_IMAGE_0`, or `FLASH_AREA_BOOTLOADER` for a bootloader) and the
chip's RAM (`MCU_RAM_SIZE`). `o` sorts by RAM instead of flash.

The preview of a package explains why it is in the build: the shortest
chain of dependencies leading to it, with the reason for each step,
followed by all packages that depend on it (what `newt target revdep`
reports):

```
 Why it is in the build
 boot/mynewt
 └ boot/bootutil  provides API bootloader
   └ crypto/mbedtls  if BOOTUTIL_USE_MBED_TLS
```

`newt size` does not support simulator targets.

## Repos

The **Repos** category lists the repos of the project, as `newt info`
does. Next to each is the version that `project.yml` asks for, or
*dependency* for a repo that comes with another repo's dependencies. The
preview shows the checkout: commit, branch, tags, whether files are
changed, the remote and the latest commits.

| Key | Does |
|---|---|
| `l` | the repo's branches |
| `c` | the commit history of what is checked out |
| `f` | fetch the repo's branches from origin (`git fetch`) |
| `e` | open `project.yml` in the editor |

`l` lists the branches you can switch to, most recently changed first:
origin's, and your local branches that origin does not have, marked
*local*. The branch that is checked out, and the one `project.yml` pins,
are marked:

```
 newt nimbleproj › apache-mynewt-nimble › branches of apache-mynewt-nimble
 update-ports-syscfg                              2026-10-28 │ master
 master                pinned · checked out · 2026-10-28 │
 eatt_m_chan                              local · 2026-10-23 │ Updated   2026-10-28
 1_8_0_dev                                        2024-07-02 │ Commit    bf0d7a9b9f7c
```

The preview lists the branch's latest commits; `c` shows its whole
history, with each commit's message and changed files in the preview. `f`
fetches new branches, and `/` filters the list.

`Enter` switches the repo to the branch, after asking: it checks the branch
out in that repo only, then pins it in `project.yml` as
`vers: <branch>-commit`, so that the next `newt upgrade` keeps it there.
origin's branches are checked out the way newt does (a detached checkout
of the branch's commit); a local branch is checked out itself, so you can
go on working on it. A pin to a local branch only works in this checkout.
If git refuses, e.g. because of changed files, nothing changes.

A repo that is not in `project.yml` but comes with another repo's
dependencies gets a `repository.<name>` block of its own, which overrides
the version the other repos ask for. The browser does not run
`newt upgrade`, as that also upgrades the repos that the repo depends on;
run it from the command line when you want that.

## The editor

On the first launch the browser asks which editor should open files:

```
 Which editor should open files? v vim · n nano
```

The answer is saved in `~/.newt/tui.yml` and used for `pkg.yml`,
`target.yml` and the source files of build errors (opened at the line with
`+<line>`, or `-g file:line` for VS Code). `V` changes it at any time; the
help pane shows the current editor. `Esc` on the first question uses
`$VISUAL`, `$EDITOR` or `vi` for now and asks again next time. Only
installed editors are offered; with neither vim nor nano installed, the
browser does not ask.

When you close the editor after changing a target's files, the browser
reloads the project.

## What the browser changes on disk

Only what you ask it to:

- **`target.yml`**: picking an app, BSP, build profile or loader rewrites
  only that `target.<variable>:` line and keeps comments and formatting
  (unlike `newt target set`, which regenerates the target's files).
- **The target's `syscfg.yml`**: syscfg values, the flash tool
  (`MYNEWT_DOWNLOADER`) and the default board (`MYNEWT_DEBUGGER_SN`) are
  lines in its `syscfg.vals` map; again only those lines change, and an
  emptied `syscfg.vals:` is removed.
- **`project.yml`**: switching a repo to a branch changes the repo's
  `vers:` line, or adds a `repository.<name>` block for a repo that is not
  listed yet.
- **The repos**: switching a branch checks it out (`git checkout`), and
  `f` runs `git fetch`.
- **Target create, copy, rename and delete** run the `newt target`
  commands, which write the files in newt's usual format.
- **`bin/targets/<target>/images/`**: copies of the images the browser
  creates.
- **`~/.newt/tui.yml`**: the chosen editor, and whether the boards pane is
  hidden.

Build, load and the other newt commands do what they do on the command
line.

## Requirements

`newt tui` is part of newt. Some features use outside tools when they are
installed, and explain what is missing when they are not:

| Tool | Used for |
|---|---|
| `nrfutil` with its `device` command (`nrfutil install device`) | listing boards and reading their chips; flashing when it is the flash tool |
| SEGGER J-Link software (`JLinkExe`, `JLinkRTTClient`) | flashing with `jlink`, listing boards without nrfutil, the RTT console |
| `nrfjprog` | flashing when it is the flash tool; resetting, erasing and recovering boards without nrfutil |
| `git` | the Repos category |
| vim or nano | editing files (or anything in `$EDITOR`) |

The serial console works on Linux and macOS.

## Troubleshooting

**"no boards found"**: check the USB cable and that `nrfutil device list`
lists the board, then `r` to scan again. On Linux, the J-Link USB rules
(`99-jlink.rules`, part of the J-Link software) must be installed for
non-root access.

**`L` says the flash tool is not installed**: install it, or pick an
installed one under the target's settings › flash.

**"build it first (b)" in the size view**: the target has no build output.

**A build fails with "Specified source file repos/... does not exist"**: a
repo that a package needs has not been downloaded; the errors list shows
newt's warning about missing or modified repos. Run `newt upgrade` from the
command line, then `R`.

**The chip details stay empty**: the browser could not read the chip, e.g.
because the board is not powered; the preview says why. nrfutil versions
differ in what they report, so some fields may be missing.

**The console shows nothing**: check that the app uses the UART console
(`CONSOLE_UART`) at the baud rate shown in the board list, and on boards
with several serial ports, try the next one with `p`.

## Key reference

Everywhere:

| Key | Does |
|---|---|
| `j` `k` `h` `l`, arrows, `Enter` | move and open |
| `gg` `G` `Ctrl-d` `Ctrl-u` `Ctrl-f` `Ctrl-b` | jump and page |
| `/`, `Esc` | filter, clear the filter |
| `R` | reload the project |
| `Ctrl-C` | stop the running command |
| `O`, `Esc` | show or hide the last command's output |
| `zb` | move into the boards pane, hide it, show it again |
| `V` | choose the editor |
| `g?` | open this guide |
| `?` | hide or show the help pane |
| `q` | quit |

In lists of targets and packages:

| Key | Does |
|---|---|
| `]` `[` | next / previous repo |
| `.`, `zh` | show or hide entries from other repos |
| `e` | open `target.yml` / `pkg.yml` in the editor |
| `ga` `gb` `gl` `gB`, `H` | go to app, BSP, loader, bootloader target; go back |
| `n` `y` `a` `D` | new, copy, rename, delete target |
| `b` `r` `L` `d` `c` | build, run, load, debug, clean |
| `i` `F` | create image, flash bootloader + app |
| `C` `s` `E` | console, size, last build's errors |
| `t` | run a unit test package |
| `c` | on a package: the commits that changed it |
| `zg` | hide or show the git pane |

In the syscfg settings: `Enter` change, `space` toggle 0/1, `x` remove
this target's value, `o` changed settings only, `B` Bluetooth essentials.

In the board list: `Enter` run, `s` run and make default, `r` scan again,
`p` next serial port, `h`/`Esc` cancel.

In the boards pane: `r` reset, `E` erase, `X` recover, `s` scan now,
`h`/`Esc` back to the lists, `zb` hide.

In the Repos category: `l` branches, `c` history, `f` fetch. In the
branch list: `Enter` check out and pin, `c` history, `f` fetch.

In the errors list: `Enter` open in the editor. In the size view: `o`
sort by RAM or flash. In the image list of `L`: `Enter` load, `x` delete.

In the console: `Enter` send, `↑`/`↓` earlier commands, `PgUp`/`PgDn`
scroll, `Esc` back to the browser, `Ctrl-]` close.

## For developers

The browser is built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss). The command is
registered in `newt/cli/tui_cmds.go`; everything else is in this
directory:

| File | Contents |
|---|---|
| `model.go` | the model: levels, cursors, key handling, the column view |
| `data.go` | loading the project into categories and entries, previews, target settings |
| `help.go` | the help pane |
| `panel.go` | the full screen lists used for errors, sizes and images |
| `targetops.go` | prompts, and creating, copying, renaming and deleting targets |
| `syscfg.go`, `syscfgview.go` | resolving a target's configuration, saving syscfg values, the syscfg view and the Bluetooth page |
| `downloader.go` | the flash tool setting and the check before loading |
| `devices.go`, `picker.go` | finding boards and reading their chips; the board list |
| `boards.go` | the boards pane: watching the connected boards, reset, erase and recover |
| `jobs.go`, `jobs_*.go` | running newt commands in the background and the output pane; process groups per platform |
| `boot.go` | bootloader targets, `create-image` and flash bootloader + app |
| `images.go` | the images of an app, choosing the image to load, keeping copies |
| `console.go`, `serial_*.go` | the console pane over a serial port or RTT; serial port setup per platform |
| `errors.go` | reading build output, the errors list, opening the editor at a line |
| `size.go` | `newt size` and the dependency chains |
| `repos.go` | the Repos category: git state, branches, history, pinning in `project.yml` |
| `gitpane.go` | the git pane under the lists of packages |
| `editor.go` | choosing and remembering the editor |
| `guide.go` | this README, built into newt and shown with `g?` |
| `exec.go` | running newt and other programs, `Run()` |
| `styles.go` | colors (the 16 ANSI colors, so the browser follows the terminal's theme) |

Design points:

- **Resolving a target** (`resolveSyscfg`) runs newt's own resolver in
  process, like `newt target config show`. It gives the syscfg settings,
  the dependency graph, the flash slot and the RAM size, and is cached
  until the next reload.
- **newt commands** run as child processes of the same binary, so the
  browser never shares newt's global state with a build. Output that the
  browser reads (build errors, sizes, newt errors) is captured.
- **Outside tools** (nrfutil, J-Link, git) are run through package
  variables such as `runTool` and `lookPath`, so that tests can replace
  them.
- **Slow work** (board scans, chip reads, `newt size`, background newt
  commands) runs in `tea.Cmd`s and reports back with messages, so the
  screen stays responsive.

### Dependencies and licenses

Besides newt's existing dependencies, the browser uses these Go modules.
All of them are under licenses that the ASF allows to be included (MIT or
BSD-3-Clause, category A). Like newt's other Go dependencies, they are
downloaded when newt is built and are not part of the source tree, so they
are not listed in newt's `LICENSE` and `NOTICE` files.

| Module | License | Used for |
|---|---|---|
| `github.com/charmbracelet/bubbletea` | MIT | the terminal UI framework |
| `github.com/charmbracelet/lipgloss` | MIT | layout and colors |
| `github.com/charmbracelet/x/ansi`, `x/cellbuf`, `x/term`, `colorprofile` | MIT | measuring and truncating text with escape sequences |
| `github.com/muesli/termenv`, `muesli/ansi`, `muesli/cancelreader` | MIT | terminal capabilities, reading input |
| `github.com/lucasb-eyer/go-colorful`, `aymanbagabas/go-osc52/v2`, `xo/terminfo` | MIT | used by Lip Gloss and termenv |
| `github.com/mattn/go-isatty`, `mattn/go-runewidth`, `rivo/uniseg` | MIT | terminal detection, character widths |
| `github.com/mattn/go-localereader`, `erikgeiser/coninput` | MIT | reading input on Windows (go-localereader states its license in its README) |
| `golang.org/x/sys`, `golang.org/x/term`, `golang.org/x/text` | BSD-3-Clause | serial ports (x/sys); terminal and text handling in Bubble Tea. x/sys and x/term were already indirect dependencies of newt |

Run the tests with:

```
go test ./newt/tui/
```

The tests drive the model with key presses and check what it does and
shows, with fake projects, boards and tools. `console_linux_test.go` tests
the serial console against a pseudo terminal, so it runs on Linux only.
