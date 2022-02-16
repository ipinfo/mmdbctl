#!/bin/bash

# Build binary for mmdbctl.

DIR=`dirname $0`
ROOT=$DIR/..

go build                                                                \
    -o $ROOT/build/                                                     \
