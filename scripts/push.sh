#!/bin/bash

DOCUMENT="1_4OtBmq2gG8zFnqTlAvpHc1sshfkv4hw3z62vHs4crI" # sample for tinkering
#DOCUMENT="1px3ivo6aFqAi0TA4u9oJkxwsry1D5GYv76GZ4nV00Rk" # ground of optimization

./doc-publisher push googledoc \
    --document $DOCUMENT
