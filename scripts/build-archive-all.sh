#!/bin/bash

# Builds cli for all platforms and packages them for release.

set -e

DIR=`dirname $0`
ROOT=$DIR/..

VSN=$1

if [ -z "$VSN" ]; then
    echo "require version as parameter" 2>&1
    exit 1
fi

# build
rm -f $ROOT/build/mmdbctl_${VSN}*
$ROOT/scripts/build-all-platforms.sh "$VSN"

# archive
cd $ROOT/build
for t in mmdbctl_${VSN}_* ; do
    if [[ $t == mmdbctl_*_windows_* ]]; then
        zip -q ${t/.exe/.zip} $t
    else
        tar -czf ${t}.tar.gz $t
    fi
done
cd ..

# dist: debian
rm -rf $ROOT/dist/usr
mkdir -p $ROOT/dist/usr/local/bin
cp $ROOT/build/mmdbctl_${VSN}_linux_amd64 dist/usr/local/bin/mmdbctl
dpkg-deb -Zgzip --build $ROOT/dist build/mmdbctl_${VSN}.deb
