Installing Newt on Linux
------------------------

You can install the latest release of the newt tool from https://mynewt.apache.org/download or download latest sources
directly from GitHub https://github.com/apache/mynewt-newt and build your binary localy.

   .. code-block:: console

    $ git clone https://github.com/apache/mynewt-newt.git


Installing Newt from Sources
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

The newt tool is written in Go (https://golang.org/). In order to build Apache Mynewt, you must have Go 1.12 or later
installed on your system. Please visit the Golang website for more information on installing Go (https://golang.org/).

#. Run the build.sh to build the newt tool.

   .. code-block:: console

    $ cd mynewt-newt
    $ ./build.sh

#. Add the path to the newt tool to your PATH.

   .. code-block:: console

    $ mkdir ~/bin
    $ mv ./newt/newt ~/bin/.
    $ export PATH=$PATH:~/bin

#. To make newt visible in every new terminal add the export to ~/.bashrc

   .. code-block:: console

    $ echo 'export PATH=$PATH:~/bin' >> ~/.bashrc

Checking the Installed Version of Newt
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

1. Check which newt you are using and that the version is the latest release version.

   .. code-block:: console

    $ which newt
    /usr/bin/newt
    $ newt version
    Apache Newt 1.13.0 / c6bf556 / 2024-11-15_06:58

2. Get information about newt:

   .. code-block:: console

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
      apropos      Search manual page names and descriptions
      build        Build one or more targets
      clean        Delete build artifacts for one or more targets
      completion   Generate the autocompletion script for the specified shell
      create-image Add image header to target binary
      debug        Open debugger session to target
      docs         Project documentation generation commands
      help         Help about any command
      info         Show project info
      load         Load built target to board
      man          Browse the man-page for given argument
      man-build    Build man pages
      mfg          Manufacturing flash image commands
      new          Create a new project
      pkg          Create and manage packages in the current workspace
      resign-image Obsolete
      run          build/create-image/download/debug <target>
      size         Size of target components
      target       Commands to create, delete, configure, and query targets
      test         Executes unit tests for one or more packages
      upgrade      Upgrade project dependencies
      vals         Display valid values for the specified element type(s)
      version      Display the Newt version number

    Flags:
          --escape            Apply Windows escapes to shell commands
      -h, --help              Help for newt commands
      -j, --jobs int          Number of concurrent build jobs (default 4)
      -l, --loglevel string   Log level (default "WARN")
      -o, --outfile string    Filename to tee output to
      -q, --quiet             Be quiet; only display error output
          --shallow int       Use shallow clone for git repositories up to specified number of commits
      -s, --silent            Be silent; don't output anything
      -v, --verbose           Enable verbose output when executing commands

    Use "newt [command] --help" for more information about a command.
