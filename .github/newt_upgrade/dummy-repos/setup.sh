#!/bin/sh
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

# Creates the dummy repos used by the newt upgrade tests from their
# fast-export streams and makes git resolve the placeholder URL used in
# the test projects to them.  Modifies the global git config.

URL_PLACEHOLDER='https://dummy.invalid/'

if [ $# -ne 1 ]
then
    echo "usage: setup.sh <dest-dir>" >&2
    exit 1
fi

set -e

SRC_DIR=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$1"
DEST_DIR=$(cd "$1" && (pwd -W 2>/dev/null || pwd))

for f in "$SRC_DIR"/*.fi
do
    repo="$DEST_DIR/$(basename "$f" .fi).git"
    echo "Creating $repo"
    git init -q --bare "$repo"
    git -C "$repo" fast-import --quiet < "$f"
    git -C "$repo" symbolic-ref HEAD refs/heads/master
done

# On Windows the path starts with a drive letter instead of a slash.
case "$DEST_DIR" in
    /*) ;;
    *) DEST_DIR="/$DEST_DIR" ;;
esac

git config --global url."file://$DEST_DIR/".insteadOf "$URL_PLACEHOLDER"
