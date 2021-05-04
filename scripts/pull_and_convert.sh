#!/bin/bash

DOCUMENT="$(cat "$1")"
FILENAME="$(basename "$1" .docid)"

doc-publisher fetch googledoc "$DOCUMENT" \
    -o "out/${FILENAME}.googledoc"

doc-publisher export markdown "out/${FILENAME}.googledoc" \
    -o "out/${FILENAME}.md"

echo
echo "================================================================================"
echo

cat "out/${FILENAME}.md"

