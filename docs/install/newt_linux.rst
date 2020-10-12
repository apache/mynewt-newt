Installing Newt on Linux
------------------------

You can install the latest release (1.8.0) of the newt tool by downloading and installing a binary executable (amd64). You can also download
and build the latest release version of newt from source.

This page shows you how to:

1. Install the latest release version of newt by manually downloading and installing the binary executable.

2. Download, build, and install the latest release version of newt from source.

If you are installing on an amd64 platform, we recommend that you install the binary executable.

See :doc:`prev_releases` to install an earlier version of newt.

**Note:** See :doc:`../../misc/go_env` if you want to:

- Use the newt tool with the latest updates from the master branch. The master branch may be unstable and we recommend
  that you use the latest stable release version.
- Contribute to the newt tool.

Installing the Latest Release of Newt
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

#. Download and unpack the newt binary executable.

   .. code-block:: console

    $ wget -P /tmp https://downloads.apache.org/mynewt/apache-mynewt-1.8.0/apache-mynewt-newt-bin-linux-1.8.0.tgz
    $ tar -xzf /tmp/apache-mynewt-newt-bin-linux-1.8.0.tgz

#. Move the executable to /usr/bin or a directory in your PATH:

   .. code-block:: console

    $ mv /tmp/apache-mynewt-newt-bin-linux-1.8.0/newt /usr/bin

See `Checking the Installed Version of Newt`_ to verify that you are using the installed version of newt.

Installing the Latest Release of Newt from a Source Package
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

If you are running Linux on a different architecture, you can build and install the latest release version of newt from
source.

#. Download and unpack the newt source:

   .. code-block:: console

    $ wget -P /tmp https://github.com/apache/mynewt-newt/archive/mynewt_1_8_0_tag.tar.gz
    $ tar -xzf /tmp/mynewt_1_8_0_tag.tar.gz

#. Run the build.sh to build the newt tool.

   .. code-block:: console

    $ cd mynewt-newt-mynewt_1_8_0_tag
    $ ./build.sh
    $ rm /tmp/mynewt_1_8_0_tag.tar.gz

#. You should see the ``newt/newt`` executable. Move the executable to a bin directory in your PATH:

   -  If you previously built newt from the master branch, you can move the binary to your $GOPATH/bin directory.

      .. code-block:: console

       $ mv newt/newt $GOPATH/bin

   -  If you are installing newt for the first time and do not have a Go workspace set up, you can move the binary to
      /usr/bin or a directory in your PATH:

      .. code-block:: console

       $ mv newt/newt /usr/bin

Checking the Installed Version of Newt
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

1. Check which newt you are using and that the version is the latest release version.

   .. code-block:: console

    $ which newt
    /usr/bin/newt
    $ newt version
    Apache Newt version: 1.8.0

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
      -j, --jobs int          Number of concurrent build jobs (default 8)
      -l, --loglevel string   Log level (default "WARN")
      -o, --outfile string    Filename to tee output to
      -q, --quiet             Be quiet; only display error output
      -s, --silent            Be silent; don't output anything
      -v, --verbose           Enable verbose output when executing commands
    
    Use "newt [command] --help" for more information about a command.
