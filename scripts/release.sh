#!/bin/bash

# Build and upload (to GitHub) for all platforms for version $1.

set -e

DIR=`dirname $0`
ROOT=$DIR/..

VSN=$1

if [ -z "$VSN" ]; then
    echo "require version as parameter" 2>&1
    exit 1
fi

# build & archive
$ROOT/scripts/build-archive-all.sh "$VSN"

# release
gh release create mmdbctl-${VSN}                                               \
    -R ipinfo/mmdbctl                                                          \
    -t "mmdbctl-${VSN}"                                                        \
    $ROOT/build/mmdbctl_${VSN}*.tar.gz                                         \
    $ROOT/build/mmdbctl_${VSN}*.zip                                            \
    $ROOT/build/mmdbctl_${VSN}*.deb                                            \
    $ROOT/macos.sh                                                             \
    $ROOT/windows.ps1                                                          \
    $ROOT/deb.sh
