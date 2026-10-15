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
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.
#
-->

# Dummy repos for newt upgrade tests

The `*.fi` files are `git fast-export` streams of the repos used by the
`newt upgrade` tests. The test projects refer to them with the
placeholder URL `https://dummy.invalid/<repo>.git`.

The tests in `newt/upgrade_test.go` import the streams into a temporary
directory and map the placeholder URL to it, so they need only `git`:

    go test -run TestUpgrade ./newt

`setup.sh <dest-dir>` does the same for running newt by hand. It imports
the streams as bare repos into `<dest-dir>` and sets a global git
`insteadOf` rule that maps the placeholder URL to them. To avoid touching
your git config, point `GIT_CONFIG_GLOBAL` at a scratch file first:

    export GIT_CONFIG_GLOBAL=/tmp/dummy-gitconfig
    newt/testdata/upgrade/dummy-repos/setup.sh /tmp/dummy-repos
    cd newt/testdata/upgrade/success/basic01
    newt upgrade

Commit hashes are deterministic, so `expected.txt` files and `-commit`
versions in the test projects can refer to them.

## Modifying a repo

Clone it from the imported bare repo, commit, tag and branch as needed,
push the changes back, then regenerate its stream:

    git -C /tmp/dummy-repos/mynewt-dummy-repo-01.git fast-export \
        --signed-tags=strip --branches --tags > mynewt-dummy-repo-01.fi

Update any `expected.txt` that depends on changed commits.
