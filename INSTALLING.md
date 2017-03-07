# Installing Newt

There are two methods for the installation of newt, either from source, or
from a binary release.  This document contains information on how to do both.

# Downloading Binary Newt

Binary releases of newt will be published on the Apache Mynewt website
when available.  To find these, please go to https://mynewt.apache.org/.

# Installing From Source

The newt tool is written in Go (https://golang.org/).  In order to build Apache
Mynewt, you must have Go 1.5 or later installed on your system.  Please visit
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
  build        Build one or more targets
  clean        Delete build artifacts for one or more targets
  create-image Add image header to target binary
  debug        Open debugger session to target
  info         Show project info
  install      Install project dependencies
  load         Load built target to board
  mfg          Manufacturing flash image commands
  new          Create a new project
  pkg          Create and manage packages in the current workspace
  run          build/create-image/download/debug <target>
  size         Size of target components
  sync         Synchronize project dependencies
  target       Commands to create, delete, configure, and query targets
  test         Executes unit tests for one or more packages
  upgrade      Upgrade project dependencies
  vals         Display valid values for the specified element type(s)
  version      Display the Newt version number

Flags:
  -h, --help              Help for newt commands
  -j, --jobs int          Number of concurrent build jobs (default 8)
  -l, --loglevel string   Log level (default "WARN")
  -o, --outfile string    Filename to tee output to
  -q, --quiet             Be quiet; only display error output
  -s, --silent            Be silent; don't output anything
  -v, --verbose           Enable verbose output when executing commands

Use "newt [command] --help" for more information about a command.


```
