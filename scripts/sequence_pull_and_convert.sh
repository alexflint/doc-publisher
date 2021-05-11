#!/bin/bash

DOCUMENT="$(cat "$1")"
FILENAME="$(basename "$1" .docid)"

doc-publisher fetch googledoc "$DOCUMENT" \
    -o "out/${FILENAME}.googledoc" || exit 1

doc-publisher export markdown "out/${FILENAME}.googledoc" \
    --separateby pagebreak \
    -o "out/${FILENAME}_part_INDEX.md"
