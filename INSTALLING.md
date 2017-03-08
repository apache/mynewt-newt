# Installing Newt

There are two methods for the installation of newt, either from source, or
from a binary release.  This document contains information on how to do both.

# Downloading Binary Newt

Binary releases of newt will be published on the Apache Mynewt website
when available.  To find these, please go to https://mynewt.apache.org/.

# Installing From Source

The newt tool is written in Go (https://golang.org/).  In order to build Apache
Mynewt, you must have Go 1.7 or later installed on your system.  Please visit
the Golang website for more information on installing Go (https://golang.org/).

Once you have Go installed, you can build newt by running the contained
build.sh script.  This script will create the newt executable with the
following path, relative to the source directory:

```no-highlight
    newt/newt
```

If you do not wish to run the included script, you can build newt manually with Go as follows (executed from the source directory):

```no-highlight
    $ mkdir "$GOPATH"/src/mynewt.apache.org
    $ cp -r * "$GOPATH"/src/mynewt.apache.org     # Or untar to this path
    $ go install mynewt.apache.org/newt/newt
```

This puts the newt binary in $GOPATH/bin

You can test the installation by running newt:

```no-highlight
$ newt
Newt allows you to create your own embedded application based on the Mynewt
operating system. Newt provides both build and package management in a single
tool, which allows you to compose an embedded application, and set of
projects, and then build the necessary artifacts from those projects. For more
information on the Mynewt operating system, please visit
https://mynewt.apache.org/.

Please use the newt help command, and specify the name of the command you want
help for, for help on how to use a specific command

Usage:
  newt [flags]
  newt [command]

Examples:
  newt
  newt help [<command-name>]
    For help on <command-name>.  If not specified, print this message.

Available Commands:
  version      Display the Newt version number.
  install      Install project dependencies
  upgrade      Upgrade project dependencies
  new          Create a new project
  info         Show project info
  target       Command for manipulating targets
  build        Builds one or more apps.
  clean        Deletes app build artifacts.
  test         Executes unit tests for one or more packages
  load         Load built target to board
  debug        Open debugger session to target
  size         Size of target components
  create-image Add image header to target binary
  run          build/create-image/download/debug <target>

Flags:
  -h, --help              help for newt
  -l, --loglevel string   Log level, defaults to WARN. (default "WARN")
  -o, --outfile string    Filename to tee log output to
  -q, --quiet             Be quiet; only display error output.
  -s, --silent            Be silent; don't output anything.
  -v, --verbose           Enable verbose output when executing commands.

Use "newt [command] --help" for more information about a command.
```
