newt debug
-----------

Open a debugger session to a target.

Usage:
^^^^^^

.. code-block:: console

        newt debug <target-name> [flag]

Flags:
^^^^^^

.. code-block:: console

          --extrajtagcmd string   Extra commands to send to JTAG software
      -n, --noGDB                 Do not start GDB from command line

Global Flags:
^^^^^^^^^^^^^

.. code-block:: console

        -h, --help              Help for newt commands
        -j, --jobs int          Number of concurrent build jobs (default 8)
        -l, --loglevel string   Log level (default "WARN")
        -o, --outfile string    Filename to tee output to
        -q, --quiet             Be quiet; only display error output
        -s, --silent            Be silent; don't output anything
        -v, --verbose           Enable verbose output when executing commands

Description
^^^^^^^^^^^

Opens a debugger session to the image built for the <target-name> target.

Examples
^^^^^^^^

+--------------------------+--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
| Usage                    | Explanation                                                                                                                                                                                                                                                                                |
+==========================+============================================================================================================================================================================================================================================================================================+
| ``newt debug myble2``    | Opens a J-Link connection and starts a GNU gdb session to debug bin/targets/myble2/app/apps/bletiny/bletiny.elf when the target is as follows: targets/myble2 app=\@apache-mynewt-core/apps/bletiny bsp=\@apache-mynewt-core/hw/bsp/nrf52dk build_profile=optimized syscfg=STATS_NAMES=1   |
+--------------------------+--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
| ``newt debug myble2 -n`` | Opens a J-Link connection bin/targets/myble2/app/apps/bletiny/bletiny.elf but do not start GDB on the command line.                                                                                                                                                                        |
+--------------------------+--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
