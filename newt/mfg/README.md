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

# mfg

### Definitions:

| Term | Long Name | Meaning |
| ---- | --------- | ------- |
| Flashdev | Flash device | A single piece of flash hardware.  E.g., "internal flash", or "external SPI flash". |
| Mfgimage | Manufacturing image | A set of files describing the entire contents of a single flashdev. At manufacturing time, a separate mfgimage is created for each of the device's flashdevs. |
| Mfgimage main binary | The binary file in an mfgimage that actually gets written to a flashdev. |
| MMR | Manufacturing Meta Region | A chunk of read-only data included in an mfgimage. Contains identifying information for the mfgimage and other data that stays with the device until end of life. |

### Manifest

Each mfgimage contains a `manifest.json` file.  A manufacturing manifest contains metadata describing the mfgimage.

#### Top-level

Manufacturing manifests are formatted as a JSON object consisting of key-value pairs.  Complex entries are further described in follow-up tables.

| Key           | Description |
| ------------- | ----------- |
| `name`        | Name of mfgimage (informational). |
| `build_time`  | Time mfgimage was created (informational). |
| `format`      | The format version of the mfgimage binary.  The current version is 2 |
| `mfg_hash`    | The SHA256 of the mfgimage binary.  To verify this hash, the embedded hash (if any) must be zeroed out prior to the calculation. |
| `version`     | The version number of this particular mfgimage (informational). |
| `device`      | The integer index of the flash device this mfgimage is intended for. |
| `bin_path`    | The relative path of the main binary within the mfgimage. |
| `hex_path`    | The relative path of the hex version of the main binary within the mfgimage. |
| `bsp`         | The name of the BSP package that mfgimage was build for. |
| `signatures`  | If the mfgimage is signed, this is an array of all the signatures. |
| `flash_map`   | The BSP flash map at the time the mfgimage was created. |
| `targets`     | An array of entries, each corresponding to a Mynewt target that is present in the mfgimage. |
| `meta`        | A set of key-value pairs describing the manufacturing meta region (MMR) |

#### Signatures

Mfgimages are signed to ensure integrity.  Typically, Mynewt images embedded in the mfgimage are individually signed, so are already protected.  The rest of the data in the mfgimage--boot loader, MMRs, configuration data, etc--is not individually signed. It is this second class of data that is protected by mfgimage signatures.

To sign an mfgimage, use the private key to sign the `mfg_hash`.

The signatures element is an array of objects, each consisting of the following key-value pairs:

| Key           | Description |
| ------------- | ----------- |
| key           | The first four bytes of the SHA256 of the public key (hex string format). |
| sig           | The full signature (hex string format). |

#### Flash Map

Every mfgimage contains a flash map in its manifest.  This information is present, even the MMR does not actually contain a flash map.

In the manifest, the `flash_map` entry is an array of flash area objects, each consisting of the following key-valur pairs:

| Key           | Description |
| ------------- | ----------- |
| `name`        | The name of the flash area, as specified in `bsp.yml`. |
| `id`          | The numeric ID of the flash area. |
| `device`      | The numeric identifer of the flash device where the area resides. |
| `offset`      | The offset of the start of the flash area within its flash device. |
| `size`        | The size of the flash area, in bytes. |

#### Targets

Most mfgimages contain at least one target.  A target can be either of 1) boot loader, or 2) Mynewt image.  In the manifest, the `targets` entry indicates the targets present in the mfgimage.  This entry is an array of target objects, each consisting of the following key-value pairs:

| Key           | Description |
| ------------- | ----------- |
| `name`        | The name of the target, as specified in its `pkg.yml` file. |
| `offset`      | The offset of the target binary within the mfgimage main binary. |
| `image_path`  | Only present for image targets.  The relative path of the Mynewt image file. |
| `bin_path`    | Only present for non-image targets.  The relative path of the target binary. |
| `manifest_path` | The relative path of the target manifest. |

#### Meta

The `meta` manifest object describes the contents of an mfgimage's MMR.  It consists of the following key-value pairs:

| Key                   | Description |
| --------------------- | ----------- |
| `end_offset`          | One past the end of the MMR region, within the mfgimage main binary. |
| `size`                | The size of the MMR, in bytes. |
| `hash_present`        | `true` if the MMR contains a hash TLV; `false` otherwise. |
| `flash_map_present`   | `true` if the MMR contains a set of flash area TLVs; `false` otherwise. |
| `mmrs`                | An array of references to external MMRs. |


### Design

#### artifact/mfg

The `artifact` library defines the `Mfg` type.  An `Mfg` is a barebones representation of an mfgimage.  It contains a raw binary of everything except the MMR, and a slice of MMR TLVs.  An `Mfg` is be converted to a flat byte slice with the following sequence:

1. Calculate the SHA256 and add it to the MMR (`Mfg#CalcHash()`)
2. Extract the byte-slice representation (`Mfg#Bytes()`)

An `Mfg` can be parsed from a byte slice using the `Parse()` function.

#### newt/mfg

##### High level

The newt tool creates mfgimages from:

1. An mfg definition (including an `mfg.yml` file).
2. Build artifacts for each target specified in the `mfg.yml` file.

Therefore, mfgimage creation typically goes something like this:

1. Build boot loader: `newt build <...>`
2. Create [signed, encrypted] images: `newt create-image -2 <...> 1.2.3.4`
3. Build mfgimage: `newt mfg create <...>`

An mfgimage created by newt consists of:

1. The binary flashdev contents.
2. A `manifest.json` file describing the mfgimage.
3. Build artifacts that were used as inputs.

##### Details

Newt performs a sequence of data structure transformations to produce the outputs listed above.  In the sequence depicted below, objects are enclosed in [brackets], steps are enclosed in (parentheses).

```
          (decode)          (build)          (emit)
[MfgDecoder] --> [MfgBuilder] --> [MfgEmitter] --> [OUTPUT]
```

The steps are described below.

###### 1. Decode

Newt parses and verifies the `mfg.yml` file.

###### 2. Build

Newt uses the output of the decode step to determine which targets are included in the mfgimage.  It ensures the targets have been built and that they all share the same BSP.  Finally, it produces an `artifact/Mfg` object from the necessary binary files.

###### 3. Emit

Newt produces the manifest and writes all the mfgimage files to disk.

### File structure

Below is an example of an mfgimage's file structure:

```
bin/mfgs/<mfg-name>/
├── manifest.json
├── mfgimg.bin
├── mfgimg.hex
└── targets
    ├── 0
    │   ├── binary.bin
    │   ├── elf.elf
    │   ├── image.hex
    │   └── manifest.json
    ├── 1
    │   ├── elf.elf
    │   ├── image.hex
    │   ├── image.img
    │   └── manifest.json
    └── <N>
        ├── <...>
      <...>
```

Each of these files is described below:

| Filename | Description |
| --------- | ----------- |
| `manifest.json` | JSON file describing the mfgimage contents. |
| `mfgimg.bin` | The mfgimage binary.  This gets written to a Mynewt device. |
| `mfgimg.hex` | The hex version of the mfgimage binary.  This gets written to a Mynewt device. |
| `targets` | A directory containing information about each target embedded in the mfgimage. |
| `0/1/N` | Correponds to an individual target.  Targets are numbered in the order they appear in `mfg.yml`. |
| `x/binary.bin` | Only present for boot loader targets.  Contains the boot loader binary generated from the target. |
| `x/elf.elf` | The ELF file corresponding to the target binary. |
| `x/image.img` | Only present for non-boot targets.  Contains the image generated from the target. |
| `x/image.hex` | Contains the hex version of the image or boot loader binary generated from the target. |
| `x/manifest.json` | JSON file describing the target. |
