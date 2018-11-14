Command Structure
-----------------

.. toctree::
   :hidden:
   :titlesonly:
   :glob:

   command_list/*

Just like verbs are actions in a sentence and adverbs modify verbs, so
in the *newt* tool, commands are actions and flags modify actions. A
command can have subcommands. Arguments to commands and subcommands,
with appropriate flags, dictate the execution and result of a command.

For instance, in the example below, the *newt* command has the
subcommand ``target set`` in which the argument 'my_target1' is the
target whose attribute, *app*, is set to ``@apache-mynewt-core/hw/bsp/nrf52dk``

.. code-block:: console

        newt target set my_target1 app=@apache-mynewt-core/hw/bsp/nrf52dk

Global flags work uniformly across *newt* commands. Consider the flag
``-v, --verbose,`` It works both for command and subcommands, to
generate verbose output. Likewise, the help flag ``-h`` or ``--help,``
to print helpful messsages.

A command may additionally take flags specific to it. For example, the
``-n`` flag instructs ``newt debug`` not to start GDB from command line.

.. code-block:: console

        newt debug <target-name> -n

In addition to the documentation in :doc:`index`,
command-line help is available for each command (and subcommand),
through the ``-h`` or ``--help`` options.

.. code-block:: console

        newt target  --help
        Commands to create, delete, configure, and query targets

        Usage:
          newt target [flags]
          newt target [command]

        Available Commands:
          amend       Add, change, or delete values for multi-value target variables
          config      View or populate a target's system configuration
          copy        Copy target
          create      Create a target
          delete      Delete target
          dep         View target's dependency graph
          revdep      View target's reverse-dependency graph
          set         Set target configuration variable
          show        View target configuration variables

        Global Flags:
          -h, --help              Help for newt commands
          -j, --jobs int          Number of concurrent build jobs (default 8)
          -l, --loglevel string   Log level (default "WARN")
          -o, --outfile string    Filename to tee output to
          -q, --quiet             Be quiet; only display error output
          -s, --silent            Be silent; don't output anything
          -v, --verbose           Enable verbose output when executing commands

        Use "newt target [command] --help" for more information about a command.


It is possible to use *newt* to debug multiple boards simultaneously. To do that, you'll need to select TCP port gdb uses to talk with JTAG adapter software, and also pick the JTAG adapter to use. Selected TCP ports should be unique, and available for use. These can be passed within parameter ``-extrajtagcmd`` wnen invoking ``newt load``, ``newt debug`` or ``newt run``.

Here are few examples:

.. code-block:: console

        newt debug blinky_nrf52 --extrajtagcmd="-port 3338 -select usb=682223238"
	newt run slinky_zero 4.0 --extrajtagcmd="cmsis_dap_serial 00000000AZE000001422 -port 3400"
