newt info 
----------

Show information about the current project.

Usage:
^^^^^^

.. code-block:: console

        newt info [flags]
        newt info --porcelain|--json [--brief] [<target-name> ...]

Flags:
^^^^^^

.. code-block:: console

            --brief       With --porcelain or --json, don't resolve the targets' builds (no syscfg or dependencies); much faster
            --json        Print the project info as JSON
            --porcelain   Print the project info as stable, line-based text for scripts
        -r, --remote      Fetch latest repos to determine if upgrades are required

Global Flags:
^^^^^^^^^^^^^

.. code-block:: console

        -h, --help              Help for newt commands
        -j, --jobs int          Number of concurrent build jobs (0 = number of CPUs) (default 8)
        -l, --loglevel string   Log level (default "WARN")
        -o, --outfile string    Filename to tee output to
        -q, --quiet             Be quiet; only display error output
        -s, --silent            Be silent; don't output anything
        -v, --verbose           Enable verbose output when executing commands

Description
^^^^^^^^^^^

Displays the repositories in the current project (the local as well as all the external repositories fetched). It also displays the packages in the local repository.

Output for scripts
^^^^^^^^^^^^^^^^^^

``--porcelain`` and ``--json`` describe the whole project in a format meant
for scripts and other tools rather than people, e.g. a terminal browser
for newt projects that cannot depend on newt itself.  Only the requested
format is written to standard output; errors go to standard error, with a
non-zero exit status.  Both formats are stable: new fields may be added,
but existing ones are not renamed or removed without increasing
``format_version``.

The description covers the repositories, all targets, with their complete
syscfg and the packages of their builds, and all packages.  Paths are
relative to the project directory, except for the path of the project.
Resolving the syscfg of every target takes most of the time, about a
second in a project with a few dozen targets, so it can be narrowed down:

* Target names limit the targets to those, e.g. ``newt info --porcelain
  blinky``.  Names are resolved as ``newt build`` resolves them.
* ``--brief`` does not resolve the builds of the targets.  Their
  ``resolved`` field is false and ``boot_loader``, ``flash_area``,
  ``syscfg``, ``syscfg_errors``, ``syscfg_warnings`` and ``deps`` are empty;
  in porcelain, these lines are left out.  Everything else is the same.

A tool that shows the whole project can read it with ``--brief`` first and
then read single targets, or all of them, when it needs their builds.

Porcelain format
~~~~~~~~~~~~~~~~

``--porcelain`` prints one value per line::

    <kind> <name> <key> <value>

``<kind>`` is ``project``, ``repo``, ``target`` or ``package``, and
``<name>`` the name of the project, repository, target or package.  The
first three fields never contain spaces; the value is the rest of the line,
with backslashes and newlines escaped as ``\\`` and ``\n``.  Lines with
empty values are left out.  Keys of lists are repeated once per element, in
order; lists of single words, e.g. ``choices``, are one line with the words
separated by spaces.

.. code-block:: console

    $ newt info --porcelain
    project my_project format_version 1
    project my_project newt_version 1.16.0
    project my_project path /home/me/my_project
    repo apache-mynewt-core path repos/apache-mynewt-core
    repo apache-mynewt-core local false
    repo apache-mynewt-core origin.type github
    repo apache-mynewt-core origin.url https://github.com/apache/mynewt-core.git
    repo apache-mynewt-core origin.user apache
    repo apache-mynewt-core origin.repo mynewt-core
    repo apache-mynewt-core requested 1.12.0
    repo apache-mynewt-core installed 1.12.0
    repo apache-mynewt-core commit c81875e23d714c2c09de722a73df0feb6bdbdad4
    repo apache-mynewt-core needs_upgrade false
    target targets/blinky repo my_project
    target targets/blinky path targets/blinky
    target targets/blinky var.app @apache-mynewt-core/apps/blinky
    target targets/blinky var.bsp @apache-mynewt-core/hw/bsp/nordic_pca10056
    target targets/blinky var.build_profile optimized
    target targets/blinky build_profiles debug default optimized speed
    target targets/blinky artifact.bin_dir bin/targets/blinky
    target targets/blinky artifact.elf bin/targets/blinky/app/@apache-mynewt-core/apps/blinky/blinky.elf
    target targets/blinky artifact.img bin/targets/blinky/app/@apache-mynewt-core/apps/blinky/blinky.img
    target targets/blinky resolved true
    target targets/blinky boot_loader false
    target targets/blinky flash_area FLASH_AREA_IMAGE_0
    target targets/blinky syscfg.LOG_LEVEL.value 1
    target targets/blinky syscfg.LOG_LEVEL.default 0
    target targets/blinky syscfg.LOG_LEVEL.description Limits what level log messages are compiled in.
    target targets/blinky syscfg.LOG_LEVEL.type raw
    target targets/blinky syscfg.LOG_LEVEL.state good
    target targets/blinky syscfg.LOG_LEVEL.package @apache-mynewt-core/sys/log/full
    target targets/blinky syscfg.LOG_LEVEL.history @apache-mynewt-core/sys/log/full 0
    target targets/blinky syscfg.LOG_LEVEL.history targets/blinky 1
    ...
    target targets/blinky dep @apache-mynewt-core/sys/console/full
    target targets/blinky dep.@apache-mynewt-core/sys/console/full.required_by @apache-mynewt-core/kernel/os api:console
    target targets/blinky dep.@apache-mynewt-core/sys/console/full.required_by @apache-mynewt-core/sys/console CONSOLE_IMPLEMENTATION == full
    ...
    package @apache-mynewt-core/hw/bsp/nordic_pca10056 type bsp
    package @apache-mynewt-core/hw/bsp/nordic_pca10056 repo apache-mynewt-core
    package @apache-mynewt-core/hw/bsp/nordic_pca10056 path repos/apache-mynewt-core/hw/bsp/nordic_pca10056
    package @apache-mynewt-core/hw/bsp/nordic_pca10056 dep @apache-mynewt-core/hw/mcu/nordic/nrf52xxx
    package @apache-mynewt-core/hw/bsp/nordic_pca10056 bsp.arch cortex_m4
    package @apache-mynewt-core/hw/bsp/nordic_pca10056 bsp.compiler compiler/arm-none-eabi-m4
    package @apache-mynewt-core/hw/bsp/nordic_pca10056 bsp.flash_area.FLASH_AREA_BOOTLOADER id=0 device=0 offset=0x0 size=32768
    package @apache-mynewt-core/hw/bsp/nordic_pca10056 bsp.flash_area.FLASH_AREA_IMAGE_0 id=1 device=0 offset=0xc000 size=483328
    ...

    $ newt info --porcelain | grep '^target .* var\.bsp '
    $ newt info --porcelain | grep '^target targets/blinky syscfg\.LOG_LEVEL\.value '

JSON format
~~~~~~~~~~~

``--json`` prints the same information as one JSON object.  Lists are
never ``null``, and strings that have no value are empty.

.. code-block:: console

    $ newt info --json
    {
      "format_version": 1,
      "newt_version": "1.16.0",
      "project": {"name": "my_project", "path": "/home/me/my_project"},
      "repos": [
        {
          "name": "apache-mynewt-core",
          "path": "repos/apache-mynewt-core",
          "local": false,
          "origin": {
            "type": "github",
            "url": "https://github.com/apache/mynewt-core.git",
            "server": "",
            "user": "apache",
            "repo": "mynewt-core",
            "branch": ""
          },
          "requested": "1.12.0",
          "installed": "1.12.0",
          "commit": "c81875e23d714c2c09de722a73df0feb6bdbdad4",
          "dirty": "",
          "needs_upgrade": false,
          "error": ""
        }
      ],
      "targets": [
        {
          "name": "targets/blinky",
          "repo": "my_project",
          "path": "targets/blinky",
          "vars": {
            "app": "@apache-mynewt-core/apps/blinky",
            "bsp": "@apache-mynewt-core/hw/bsp/nordic_pca10056",
            "build_profile": "optimized"
          },
          "build_profiles": ["debug", "default", "optimized", "speed"],
          "artifacts": {
            "bin_dir": "bin/targets/blinky",
            "elf": "bin/targets/blinky/app/@apache-mynewt-core/apps/blinky/blinky.elf",
            "img": "bin/targets/blinky/app/@apache-mynewt-core/apps/blinky/blinky.img"
          },
          "resolved": true,
          "boot_loader": false,
          "flash_area": "FLASH_AREA_IMAGE_0",
          "syscfg": [
            {
              "name": "LOG_LEVEL",
              "value": "1",
              "default": "0",
              "description": "Limits what level log messages are compiled in.",
              "type": "raw",
              "state": "good",
              "package": "@apache-mynewt-core/sys/log/full",
              "value_ref": "",
              "choices": [],
              "restrictions": [],
              "history": [
                {"source": "@apache-mynewt-core/sys/log/full", "value": "0"},
                {"source": "targets/blinky", "value": "1"}
              ]
            }
          ],
          "syscfg_errors": [],
          "syscfg_warnings": [],
          "deps": [
            {
              "name": "@apache-mynewt-core/sys/console/full",
              "required_by": [
                {
                  "name": "@apache-mynewt-core/kernel/os",
                  "syscfg": "",
                  "apis": ["console"]
                },
                {
                  "name": "@apache-mynewt-core/sys/console",
                  "syscfg": "CONSOLE_IMPLEMENTATION == full",
                  "apis": []
                }
              ]
            }
          ]
        }
      ],
      "packages": [
        {
          "name": "@apache-mynewt-core/hw/bsp/nordic_pca10056",
          "type": "bsp",
          "repo": "apache-mynewt-core",
          "path": "repos/apache-mynewt-core/hw/bsp/nordic_pca10056",
          "description": "",
          "author": "Apache Mynewt <dev@mynewt.apache.org>",
          "homepage": "http://mynewt.apache.org/",
          "keywords": ["nrf52840", "nrf52840dk", "nordic", "pca10056"],
          "deps": ["@apache-mynewt-core/hw/mcu/nordic/nrf52xxx"],
          "apis": [],
          "req_apis": [],
          "syscfg_defs": 2,
          "bsp": {
            "arch": "cortex_m4",
            "compiler": "compiler/arm-none-eabi-m4",
            "flash_map": [
              {"name": "FLASH_AREA_BOOTLOADER", "id": 0, "device": 0, "offset": 0, "size": 32768},
              {"name": "FLASH_AREA_IMAGE_0", "id": 1, "device": 0, "offset": 49152, "size": 483328}
            ]
          }
        }
      ]
    }

    $ newt info --json | jq '.targets[] | select(.name == "targets/blinky") | .syscfg[] | select(.name == "LOG_LEVEL")'

Fields
~~~~~~

**Project**: ``project <project-name> <key> <value>``; in JSON, the top-level object and its ``project``.

.. list-table::
    :header-rows: 1
    :widths: 20 25 55

    * - JSON
      - Porcelain
      - Meaning
    * - ``format_version``
      - ``format_version``
      - Version of the format; increased only when a change can break readers.
    * - ``newt_version``
      - ``newt_version``
      - Version of newt.
    * - ``project.name``
      - --
      - Name of the project, from ``project.yml``; in porcelain, the second field of every ``project`` line.
    * - ``project.path``
      - ``path``
      - Absolute path of the project directory.

**Repositories**: ``repo <repo-name> <key> <value>``; in JSON, the ``repos`` list.

.. list-table::
    :header-rows: 1
    :widths: 20 25 55

    * - JSON
      - Porcelain
      - Meaning
    * - ``name``
      - --
      - Name of the repository.
    * - ``path``
      - ``path``
      - Directory of the repository.
    * - ``local``
      - ``local``
      - ``true`` for the project itself.
    * - ``origin.type``
      - ``origin.type``
      - Where the repository is downloaded from: ``github``, ``git`` or ``local``.
    * - ``origin.url``
      - ``origin.url``
      - URL to clone, without credentials, or the directory of a local repository.
    * - ``origin.server``
      - ``origin.server``
      - GitHub server, if not ``github.com``.
    * - ``origin.user``
      - ``origin.user``
      - GitHub user or organization.
    * - ``origin.repo``
      - ``origin.repo``
      - GitHub repository name.
    * - ``origin.branch``
      - ``origin.branch``
      - Branch, if declared.
    * - ``requested``
      - ``requested``
      - Version ``project.yml`` asks for; empty for repositories only other repositories depend on.
    * - ``installed``
      - ``installed``
      - Installed version; empty if not installed.
    * - ``commit``
      - ``commit``
      - Checked out commit.
    * - ``dirty``
      - ``dirty``
      - What is modified in the working tree; empty if clean.
    * - ``needs_upgrade``
      - ``needs_upgrade``
      - Whether ``newt upgrade`` would change the repository; only known with ``--remote``.
    * - ``error``
      - ``error``
      - Why the state of the repository could not be read.

**Targets**: ``target <target-name> <key> <value>``; in JSON, the ``targets`` list.  Targets of all repositories are included; ``repo`` tells where each comes from.

.. list-table::
    :header-rows: 1
    :widths: 20 25 55

    * - JSON
      - Porcelain
      - Meaning
    * - ``name``
      - --
      - Full name of the target.
    * - ``repo``
      - ``repo``
      - Repository of the target.
    * - ``path``
      - ``path``
      - Directory of the target.
    * - ``vars``
      - ``var.<name>``
      - Variables of ``target.yml`` without the ``target.`` prefix (e.g. ``app``, ``bsp``, ``loader``, ``build_profile``) and the ``cflags``, ``cxxflags``, ``lflags`` and ``aflags`` of the target.  Lists are joined with spaces; empty variables are left out.
    * - ``build_profiles``
      - ``build_profiles``
      - Build profiles the target can use: those the compiler of its BSP defines, ``default`` and the one the target uses.  Porcelain: separated by spaces.
    * - ``artifacts.bin_dir``
      - ``artifact.bin_dir``
      - Directory of the build output of the target.
    * - ``artifacts.elf``
      - ``artifact.elf``
      - ELF file ``newt build`` creates; empty for a target without an app.  The file exists only once the target is built.
    * - ``artifacts.img``
      - ``artifact.img``
      - Image ``newt create-image`` creates; empty for a target without an app.
    * - ``resolved``
      - ``resolved``
      - Whether the build of the target was resolved, i.e. ``--brief`` was not given.  The fields below are empty if not.
    * - ``boot_loader``
      - ``boot_loader``
      - Whether the target is a bootloader, i.e. its build sets ``BOOT_LOADER``.
    * - ``flash_area``
      - ``flash_area``
      - Flash area ``newt load`` writes the target to: ``FLASH_AREA_BOOTLOADER`` or ``FLASH_AREA_IMAGE_0``.  A target that is not a bootloader needs one when the flash map of its BSP has ``FLASH_AREA_BOOTLOADER``.
    * - ``syscfg_errors``
      - ``syscfg_error``
      - Lines of the errors that would make ``newt build`` fail, as newt prints them, e.g. for a target without an app.  Porcelain: one line each.
    * - ``syscfg_warnings``
      - ``syscfg_warning``
      - Lines of the warnings ``newt build`` would print.  Porcelain: one line each.
    * - ``syscfg``
      - ``syscfg.<setting>.<field>``
      - Every syscfg setting of the build of the target, as ``newt target config show`` displays it; see below.
    * - ``deps``
      - ``dep``, ``dep.<package>.required_by``
      - Every package of the build of the target, as ``newt target revdep`` displays them; see below.

**Syscfg settings**: In JSON, the ``syscfg`` list of a target, sorted by name; in porcelain, ``target <target-name> syscfg.<setting>.<field> <value>``.

.. list-table::
    :header-rows: 1
    :widths: 20 25 55

    * - JSON
      - Porcelain
      - Meaning
    * - ``name``
      - --
      - Name of the setting.
    * - ``value``
      - ``value``
      - Value in the build of the target.
    * - ``default``
      - ``default``
      - Value from the definition of the setting.
    * - ``description``
      - ``description``
      - Description of the setting.
    * - ``type``
      - ``type``
      - ``raw``, ``task_priority`` or ``flash_owner``.
    * - ``state``
      - ``state``
      - ``good``, ``const``, ``deprecated``, ``defunct`` or ``experimental``.
    * - ``package``
      - ``package``
      - Package that defines the setting; ``newt`` for settings newt injects.
    * - ``value_ref``
      - ``value_ref``
      - Setting the value is copied from, if any.
    * - ``choices``
      - ``choices``
      - Allowed values, if restricted to a list.  Porcelain: separated by spaces.
    * - ``restrictions``
      - ``restriction``
      - Restrictions on the value.  JSON: objects with a ``type`` of ``notnull``, ``expr`` or ``range``, and the expression or ranges in ``expr``.  Porcelain: one line each, ``notnull``, ``expr <expression>`` or ``range <ranges>``.
    * - ``history``
      - ``history``
      - Where the value comes from, starting with the package that defines the setting.  JSON: objects with ``source`` and ``value``.  Porcelain: one line each, ``<package> <value>``.  ``newt`` stands for values newt itself sets.

**Dependencies**: In JSON, the ``deps`` list of a target, sorted by name; in porcelain, a ``dep <package>`` line for each package, followed by its ``dep.<package>.required_by`` lines.

.. list-table::
    :header-rows: 1
    :widths: 20 25 55

    * - JSON
      - Porcelain
      - Meaning
    * - ``name``
      - ``dep``
      - Package of the build.
    * - ``required_by``
      - ``dep.<package>.required_by``
      - Packages that pull the package in; empty for the packages the build starts from (the target, its app, its BSP, ...).  JSON: objects with ``name``, the ``syscfg`` expression the dependency depends on (empty if unconditional) and the ``apis`` it goes through.  Porcelain: one line each, ``<package> [api:<api>[,<api>...]] [<syscfg-expression>]``.

**Packages**: ``package <package-name> <key> <value>``; in JSON, the ``packages`` list.  Packages of all repositories are included.

.. list-table::
    :header-rows: 1
    :widths: 20 25 55

    * - JSON
      - Porcelain
      - Meaning
    * - ``name``
      - --
      - Full name of the package.
    * - ``type``
      - ``type``
      - ``app``, ``bsp``, ``lib``, ``sdk``, ``target``, ``unittest``, ...
    * - ``repo``
      - ``repo``
      - Repository of the package.
    * - ``path``
      - ``path``
      - Directory of the package.
    * - ``description``
      - ``description``
      - ``pkg.description`` of ``pkg.yml``.
    * - ``author``
      - ``author``
      - ``pkg.author``.
    * - ``homepage``
      - ``homepage``
      - ``pkg.homepage``.
    * - ``keywords``
      - ``keywords``
      - ``pkg.keywords``.  Porcelain: separated by spaces.
    * - ``deps``
      - ``dep``
      - ``pkg.deps``, including conditional ones.  Porcelain: one line each.
    * - ``apis``
      - ``api``
      - ``pkg.apis``.  Porcelain: one line each.
    * - ``req_apis``
      - ``req_api``
      - ``pkg.req_apis``.  Porcelain: one line each.
    * - ``syscfg_defs``
      - ``syscfg_defs``
      - Number of settings the package defines in ``syscfg.yml``.
    * - ``bsp.arch``
      - ``bsp.arch``
      - For BSPs: architecture.  ``bsp`` is ``null`` in JSON for other packages.
    * - ``bsp.compiler``
      - ``bsp.compiler``
      - For BSPs: compiler package.
    * - ``bsp.flash_map``
      - ``bsp.flash_area.<area>``
      - For BSPs: the flash areas, sorted by device and offset.  JSON: objects with ``name``, ``id``, ``device``, ``offset`` and ``size``.  Porcelain: ``id=<id> device=<device> offset=0x<offset> size=<size>``, with the size in bytes.

With ``--remote``, the remotes of the repositories are queried first, so
that ``needs_upgrade`` is known.
