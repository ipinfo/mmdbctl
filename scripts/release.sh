#!/bin/bash

# Build and upload (to GitHub) for all platforms for version $1.

set -e

VSN=$1

if [ -z "$VSN" ]; then
    echo "require version as parameter" 2>&1
    exit 1
fi

# build
rm -f build/mmdbctl_${VSN}*
scripts/build-all-platforms.sh "$VSN"

# archive
cd build
for t in mmdbctl_${VSN}_* ; do
    if [[ $t == ${CLI}_*_windows_* ]]; then
        zip -q ${t/.exe/.zip} $t
    else
        tar -czf ${t}.tar.gz $t
    fi
done
cd ..

# dist: debian
rm -rf dist/usr
mkdir -p dist/usr/local/bin
cp build/mmdbctl_${VSN}_linux_amd64 dist/usr/local/bin/mmdbctl
dpkg-deb --build dist build/mmdbctl_${VSN}.deb

# release
gh release create mmdbctl-${VSN}                                         \
    -R ipinfo/mmdbctl                                                    \
    -t "mmdbctl-${VSN}"                                                  \
    build/mmdbctl_${VSN}*.tar.gz                                         \
    build/mmdbctl_${VSN}*.zip                                            \
    build/mmdbctl_${VSN}*.deb                                            \
    macos.sh                                                             \
    windows.ps1                                                          \
    deb.sh