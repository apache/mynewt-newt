newt resign-image
------------------

Sign or re-sign an existing image file.

Usage:
^^^^^^

.. code-block:: console

        newt resign-image <image-file> [signing-key [key-id]][flags]

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

Changes the signature of an existing image file. To sign an image, specify a .pem file for the ``signing-key`` and an
optional ``key-id``. ``key-id`` must be a value between 0-255. If a signing key is not specified, the command strips the
current signature from the image file.

A new image header is created. The rest of the image is byte-for-byte equivalent to the original image.

Warning: The image hash will change if you change the key-id or the type of key used for signing.

Examples
^^^^^^^^

+------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------+
| Usage                                                                              | Explanation                                                                                   |
+====================================================================================+===============================================================================================+
| ``newt resign-image bin/targets/myble/app/apps/bletiny/bletiny.img private.pem``   | Signs the ``bin/targets/myble/app/apps/bletiny/bletiny.img`` file with the private.pem key.   |
+------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------+
| ``newt resign-image bin/targets/myble/app/apps/bletiny/bletiny.img``               | Strips the current signature from ``bin/targets/myble/app/apps/bletiny/bletiny.img`` file.    |
+------------------------------------------------------------------------------------+-----------------------------------------------------------------------------------------------+
