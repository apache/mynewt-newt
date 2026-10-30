newt tui
--------

Browse the project's targets and packages in a terminal UI.

Usage:
^^^^^^

.. code-block:: console

        newt tui [flags]
        newt --tui
        newt -t

Global Flags:
^^^^^^^^^^^^^

.. code-block:: console

          --escape            Apply Windows escapes to shell commands
      -h, --help              Help for newt commands
      -j, --jobs int          Number of concurrent build jobs (default 8)
      -l, --loglevel string   Log level (default "WARN")
      -o, --outfile string    Filename to tee output to
      -q, --quiet             Be quiet; only display error output
          --shallow int       Use shallow clone for git repositories up to specified number of commits
      -s, --silent            Be silent; don't output anything
      -v, --verbose           Enable verbose output when executing commands

Description
^^^^^^^^^^^

Opens a browser for the project in the terminal, in the style of the ranger
file manager. The project's targets, apps, BSPs, libraries and unit tests
are shown in columns with Vim-like keys (``h``, ``j``, ``k``, ``l``, ``/`` to
filter) and a preview of the selected item. A help pane at the bottom of
the screen lists the keys that work at the current position; ``?`` hides or
shows it.

From the browser you can:

* Configure a target: pick its app, BSP, build profile and flash tool from
  lists, and change its syscfg settings, with each setting's description,
  default, history and restrictions shown. Only the changed lines of
  ``target.yml`` and the target's ``syscfg.yml`` are rewritten.
* Create, copy, rename and delete targets.
* Build, load, debug, size, clean and test targets. Builds and loads run in
  the background with their output in a pane; failed builds list their
  errors, and ``Enter`` on an error opens the file at that line in the
  editor.
* Choose the board to load or debug when several are connected; the chip on
  each board is read with nrfutil and compared with the one the target is
  built for.
* For apps that run under a bootloader, choose which image to load, or
  create a new one, and flash the bootloader and the app in one step.
* Open the board's console (UART or RTT) in a pane, and keep it open while
  building and loading.
* See the connected boards in a pane, and reset, erase or recover them.
* See what each package of a build costs in flash and RAM, and why it is
  part of the build.
* See the project's repos, their branches and history, and switch a repo to
  another branch: it is checked out and pinned in ``project.yml``. A git
  pane shows the latest commits that changed the selected package.

Run ``newt tui`` from anywhere inside a project. On the first launch it asks
whether files should open in vim or nano; the choice is kept in
``~/.newt/tui.yml``.

Some features use outside tools when they are installed: nrfutil (listing
boards, reading chips, flashing), the SEGGER J-Link software (flashing, the
RTT console), nrfjprog (flashing) and git (repos and the git pane).

The full guide, with all keys, is ``newt/tui/README.md`` in the newt
repository.

Examples
^^^^^^^^

.. tabularcolumns:: |l|p{11.5cm}|
.. table::

   +-------------------+------------------------------------------------------------+
   | Usage             | Explanation                                                |
   +===================+============================================================+
   | ``newt tui``      | Opens the browser for the project in the current directory |
   +-------------------+------------------------------------------------------------+
   | ``newt -t``       | Same as ``newt tui``                                       |
   +-------------------+------------------------------------------------------------+
