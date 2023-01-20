#!/bin/sh

print_usage() {
    echo "Generates known-answer json files for the newt dump tests." >&2
    echo "Must be run from the 'newt_dump' directory." >&2
    echo >&2
    echo "usage: generate-answers.sh" >&2
}

usage_err() {
    if [ "$1" != "" ]
    then
        printf '* error: %s\n\n' "$1" >&2
        print_usage
        exit 1
    fi
}

TARGETS_DIR='proj/targets'

if [ ! -d "$TARGETS_DIR" ]
then
    usage_err "cannot find $TARGETS_DIR directory"
fi

if [ "$1" = '-h' ]
then
    print_usage
    exit 0
fi

# Run this in a subshell so that the user's PWD is preserved.
(
    cd "$TARGETS_DIR" &&
    for t in *
    do
        if ! [ "$t" = 'unittest' ]
        then
            filename="answers/$t.json"
            echo "Generating $filename"
            newt target dump "$t" | jq 'del(.sysinit)' > "../../$filename"
        fi
    done
)
