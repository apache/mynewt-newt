#!/bin/sh

set -e

### Ensure >= go1.5 is installed.
go_ver_str="$(go version | cut -d ' ' -f 3)"
go_ver="${go_ver_str#go}"
go_maj="${go_ver%.*}"
go_min="${go_ver#*.}"

if [ ! "$go_maj" -gt 1 ] && [ ! "$go_min" -ge 5 ]
then
    printf "* Error: go 1.5 or later is required (detected version: %s)\n" \
        "$go_maj"."$go_min"
    exit 1
fi

### Create a temporary go tree in /tmp.
installdir="$(realpath "$(dirname "$0")")"
godir="$(mktemp -d /tmp/mynewt.XXXXXXXXXX)"
mynewtdir="$godir"/src/mynewt.apache.org
repodir="$mynewtdir"/newt
newtdir="$repodir"/newt
dstfile="$installdir"/newt/newt

mkdir -p "$mynewtdir"
ln -s "$installdir" "$repodir"

### Build newt.
(
    cd "$newtdir"

    printf "Building newt.  This may take a minute...\n"
    GOPATH="$godir" GO15VENDOREXPERIMENT=1 go install

    mv "$godir"/bin/newt "$dstfile"

    printf "Successfully built executable: %s\n" "$dstfile"
)

### Delete the temporary directory.
rm -r "$godir"
