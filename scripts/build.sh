#!/bin/bash

# Build binaries for mmdbctl and mmdbshrink.

DIR=`dirname $0`
ROOT=$DIR/..

go build                                                                \
    -o $ROOT/build/                                                     \
    $ROOT/                                                              \
    $ROOT/mmdbshrink
