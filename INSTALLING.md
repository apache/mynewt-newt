# Installing Newt

There are two methods for the installation of newt.  From source, and 
binary releases.  This document contains information on how to do both.

# Downloading Binary Newt

Binary releases of newt will be published on the Apache Mynewt website 
when available.  To find these, please go to https://mynewt.apache.org/.

# Installing From Source

The newt tool is written in Go (https://golang.org/).  In order to build 
Apache Mynewt, you must have Go 1.6 installed on your system.  Please 
visit the Golang website for more information on installing Go.

Once you have go installed, you must install the Apache Mynewt sources 
in: 

  $GOPATH/src/mynewt.apache.org/newt 

You can do this by either using go get: 

  $ go get -v mynewt.apache.org/newt

Or manually git cloning the directory: 
  
  $ mkdir -p $GOPATH/src/mynewt.apache.org/
  $ cd $GOPATH/src/mynewt.apache.org/
  $ git clone https://github.com/apache/incubator-mynewt-newt

NOTE: To get the latest development version of newt, you should checkout the 
"develop" branch, once you've cloned newt.  The master branch represents the
latest stable newt.

Once you've done this, the next step is to install newt's dependencies, this 
can be done with the go get command: 

  $ cd $GOPATH/src/mynewt.apache.org/newt/newt
  $ go get -v 

Once you've fetched all the sources, the final step is to install newt.  To do this
issue the go install command: 

  $ go install -v

This should install the newt binary in the following location:

  $GOPATH/bin

Which should be added to your path during the installation of Go. 

You can test the installation by typing newt: 

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
$ 
```

