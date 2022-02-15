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

if which xclip > /dev/null; then
    echo "copied markdown to ctrl+c/ctrl+v clipboard"
    cat "out/${FILENAME}.md" | xclip -selection c
else
    cat "out/${FILENAME}.md"
fi
