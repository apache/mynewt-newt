newt target
------------

Commands to create, delete, configure and query targets.

Usage:
^^^^^^

.. code-block:: console

        newt target [command] [flags]

Available Commands:
^^^^^^^^^^^^^^^^^^^

.. code-block:: console

        amend       Add, change, or delete values for multi-value target variables
        config      View or populate a target's system configuration settings
        copy        Copy target
        create      Create a target
        delete      Delete target
        dep         View target's dependency graph
        revdep      View target's reverse-dependency graph
        set         Set target configuration variable
        show        View target configuration variables

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

The target command provides subcommands to create, build, delete, and query targets. The subcommands take one or two ``target-name`` arguments.

.. tabularcolumns:: |l|p{13.8cm}|
.. table::
   :class: longtable

   =============   =========================================================================================================================
   Sub-command     Explanation
   =============   =========================================================================================================================
   amend           The amend command allows you to add, change, or delete values for multi-value target variables that you
                   have set with the ``newt target set`` command. The format of the amend command is:

                   ``newt target amend <target-name> <var-name=var-value> [var-name=var-value...]``

                   Specify the ``-d`` flag to delete values.

                   The following multi-value variables can be amended: ``aflags``, ``cflags``, ``lflags``, ``syscfg``.

                   The ``var-value`` format depends on the ``var-name`` as follows:

                   ``aflags``, ``cflags``, ``lflags``:
                     A string of flags, with each flag separated by a space. These variables are saved in the target's ``pkg.yml`` file.

                   ``syscfg``:
                     The ``syscfg`` variable allows you to assign values to configuration settings in your target's ``syscfg.yml`` file. The format is:

                     ``syscfg=setting-name1=setting-value1[:setting-name2=setting-value2...]``, where ``setting-name1`` is a
                     configuration setting name and ``setting-value1`` is the value to assign to ``setting-name1``. If ``setting-value1``
                     is not specified, the setting is set to value ``1``. You use a ``:`` to delimit each setting when you amend multiple
                     settings.

                     To delete a system configuration setting, you only need to specify the setting name.
                     For example, ``syscfg=setting-name1:setting-name2``
                     deletes configuration settings named ``setting-name1`` and ``setting-name2``.

   config          The config command allows you to view or populate a target's system configuration settings.
                   A target's system configuration settings include the settings of all the packages it includes.
                   The settings for a package are listed in the package's ``syscfg.yml`` file. The ``config`` command has
                   two subcommands: ``show`` and ``init``. The config show <target-name> command displays the system
                   configuration setting definitions and values for all the packages that the ``target-name`` target includes.
                   The config init <target-name> command populates the target's ``syscfg.yml`` file with the system configuration
                   values for all the packages that the ``target-name`` target includes.

   copy            The copy <src-target> <dst-target> command creates a new target named ``dst-target`` by cloning the
                   ``src-target`` target.

   create          The create <target-name> command creates an empty target named ``target-name``. It creates the
                   ``targets/target-name`` directory and the skeleton ``pkg.yml`` and ``target.yml`` files in the directory.

   delete          The delete <target-name> command deletes the description for the ``target-name`` target. It deletes
                   the 'targets/target-name' directory. It does not delete the 'bin/targets/target-name' directory where
                   the build artifacts are stored. If you want to delete the build artifacts, run the ``newt clean <target-name>``
                   command **before** deleting the target.

   dep             The dep <target-name> command displays a dependency tree for the packages that the ``target-name``
                   target includes. It shows each package followed by the list of libraries or packages that it
                   depends on.

   revdep          The revdep <target-name> command displays the reverse dependency tree for the packages that the
                   ``target-name`` target includes. It shows each package followed by the list of libraries or packages
                   that depend on it.

   set             The set <target-name> <var-name=var-value> [var-name=var-value...] command sets variables (attributes)
                   for the <target-name> target. The set command overwrites your current variable values.

                   The valid ``var-name`` values are: ``app``, ``bsp``, ``loader``, ``build_profile``, ``cflags``,
                   ``lflags``, ``aflags``, ``syscfg``.

                   The ``var-value`` format depends on the ``var-name`` as follows:

                   ``app``, ``bsp``, ``loader``:
                     \@<source-path>, where ``source-path`` is the directory containing the application or bsp source.
                     These variables are stored in the target's target.yml file. For a simulated target, e.g. for software
                     testing purposes, set ``bsp`` to ``@apache-mynewt-core/hw/bsp/native``.

                   ``build_profile``:
                     ``optimized`` or ``debug``

                   ``aflags``, ``cflags``, ``lflags``:
                     A string of flags, with each flag separated by a space. These variables are saved in the target's ``pkg.yml`` file.

                   ``syscfg``:
                     The ``syscfg`` variable allows you to assign values to configuration settings in your target's ``syscfg.yml`` file. The format is:

                     ``syscfg=setting-name1=setting-value1[:setting-name2=setting-value2...]``, where ``setting-name1``
                     is a configuration setting name and ``setting-value1`` is the value to assign to ``setting-name1``.
                     If ``setting-value1`` is not specified, the setting is set to value ``1``. You use a ``:`` to delimit
                     each setting when you set multiple settings.

                   You can specify ``var-name=`` or ``var-name=""`` to unset a variable value.

                   **Warning**: For multi-value variables, the command overrides all existing values. Use
                   the ``newt target amend`` command to change or add new values for a multi-value variable after you have
                   set the variable value. The multi-value variables are: ``aflags``, ``cflags``, ``lflags``, and ``syscfg``

                   To display all the existing values for a target variable (attribute), you can run the ``newt vals <variable-name>``
                   command. For example, ``newt vals app`` displays the valid values available for the variable ``app`` for any target.

   show            The show [target-name] command shows the values of the variables (attributes) for the ``target-name``
                   target. When ``target-name`` is not specified, the command shows the variables for
                   all the targets that are defined for your project.

   =============   =========================================================================================================================


Examples
^^^^^^^^

.. tabularcolumns:: |l|p{6.5cm}|p{7cm}|
.. table::

   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | Sub-command   | Usage                                                   | Explanation                                                                                                                                                                                                                                           |
   +===============+=========================================================+=======================================================================================================================================================================================================================================================+
   | amend         | ``newt target amend myble``                             | Changes (or adds) the ``CONFIG_NEWTMGR`` variable to value 0 in the ``syscfg.yml`` file and adds the -DTEST flag to ``pkg.cflags`` in the ``pkg.yml`` file for the ``myble`` target. Other syscfg setting values and cflags values are not changed.   |
   |               | ``syscfg=CONFIG_NEWTMGR=0 cflags="-DTEST"``             |                                                                                                                                                                                                                                                       |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | amend         | ``newt target amend myble``                             | Deletes the ``LOG_LEVEL`` and ``CONFIG_NEWTMGR`` settings from the ``syscfg.yml`` file and the -DTEST flag from ``pkg.cflags`` for the ``myble`` target. Other syscfg setting values and cflags values are not changed.                               |
   |               | ``-d syscfg=LOG_LEVEL:CONFIG_NEWTMGR cflags="-DTEST"``  |                                                                                                                                                                                                                                                       |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | config show   | ``newt target config show rb_blinky``                   | Shows the system configuration settings for all the packages that the ``rb_blinky`` target includes.                                                                                                                                                  |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | config init   | ``newt target config init my_blinky``                   | Creates and populates the ``my_blinky`` target's ``syscfg.yml`` file with the system configuration setting values from all the packages that the ``my_blinky`` target includes.                                                                       |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | copy          | ``newt target copy rb_blinky rb_btshell``               | Creates the ``rb_btshell`` target by cloning the ``rb_blinky`` target.                                                                                                                                                                                |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | create        | ``newt target create my_new_target``                    | Creates the ``my_newt_target`` target. It creates the ``targets/my_new_target`` directory and creates the skeleton ``pkg.yml`` and ``target.yml`` files in the directory.                                                                             |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | delete        | ``newt target delete rb_btshell``                       | Deletes the ``rb_btshell`` target. It deletes the ``targets/rb_btshell`` directory.                                                                                                                                                                   |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | dep           | ``newt target dep myble``                               | Displays the dependency tree of all the package dependencies for the ``myble`` target. It lists each package followed by a list of packages it depends on.                                                                                            |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | revdep        | ``newt target revdep myble``                            | Displays the reverse dependency tree of all the package dependencies for the ``myble`` target. It lists each package followed by a list of packages that depend on it.                                                                                |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | set           | ``newt target set myble``                               | Use ``btshell`` as the application to build for the ``myble`` target.                                                                                                                                                                                 |
   |               | ``app=@apache-mynewt-core/apps/btshell``                |                                                                                                                                                                                                                                                       |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | set           | ``newt target set myble``                               | Set ``pkg.cflags`` variable with ``-DNDEBUG -Werror`` in the ``myble`` target's ``pkg.yml`` file..                                                                                                                                                    |
   |               | ``cflags="-DNDEBUG -Werror"``                           |                                                                                                                                                                                                                                                       |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | set           | ``newt target set myble``                               | Sets the ``syscfg.vals`` variable in the ``myble`` target's ``syscfg.yml`` file with the setting values: LOG_NEWTMGR: 0 and CONFIG_NEWTMGR: 1. CONFIG_NEWTMGR is set to 1 because a value is not specified.                                           |
   |               | ``syscfg=LOG_NEWTMGR=0:CONFIG_NEWTMGR``                 |                                                                                                                                                                                                                                                       |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | set           | ``newt target set myble cflags=``                       | Unsets the ``pkg.cflags`` variable in the ``myble`` target's ``pkg.yml`` file.                                                                                                                                                                        |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | show          | ``newt target show myble``                              | Shows all variable settings for the ``myble`` target, i.e. the values that app, bsp, build_profile, cflags, aflags, ldflags, syscfg variables are set to. Note that not all variables have to be set for a target.                                    |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
   | show          | ``newt target show``                                    | Shows all the variable settings for all the targets defined for the project.                                                                                                                                                                          |
   +---------------+---------------------------------------------------------+-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
