# Make a preview site for Sphinx output

BUILDDIR      = _build/html

.PHONY: Makefile clean preview

clean:
	rm -rf _build

preview: _build sphinx

_build:
	mkdir -p _build

sphinx:
	sphinx-build . ${BUILDDIR}
