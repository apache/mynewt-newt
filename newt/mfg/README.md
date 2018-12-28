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
| Mfgimage | Manufacturing image | A file with the entire contents of a single flashdev. At manufacturing time, a separate mfgimage is typically written to each of the device's flashdevs. |
| MMR | Manufacturing Meta Region | A chunk of read-only data included in an mfgimage. Contains identifying information for the mfgimage and other data that stays with the device until end of life. |

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
