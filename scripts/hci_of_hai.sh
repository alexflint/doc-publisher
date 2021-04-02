#!/bin/bash

doc-publisher fetch googledoc "13nW4gJ8sP5pak-lvddD3H1g_gbjXlEl36AXBVwQxfsg" \
    -o out/hci_of_hai.googledoc

doc-publisher export markdown out/hci_of_hai.googledoc \
    -o out/hci_of_hai.md
