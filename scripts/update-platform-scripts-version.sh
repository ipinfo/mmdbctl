#!/bin/bash

# Builds versioned platforms scripts (deb.sh, macos.sh, windows.ps1, dist/DEBIAN/control)

set -e

DIR=`dirname $0`
ROOT=$DIR/..

VSN=$1

if [ -z "$VSN" ]; then
    echo "require version as parameter" 2>&1
    exit 1
fi

# Generate new versions separately (otherwise we get an empty file)
cat $ROOT/deb.sh | sed "s/VSN=.*/VSN=${VSN}/" > $ROOT/deb.sh.new
cat $ROOT/macos.sh | sed "s/VSN=.*/VSN=${VSN}/" > $ROOT/macos.sh.new
cat $ROOT/windows.ps1 | sed "s/\$VSN =.*/\$VSN = \"${VSN}\"/" > $ROOT/windows.ps1.new
cat $ROOT/dist/DEBIAN/control | sed "s/Version: .*/Version: ${VSN}/" > $ROOT/dist/DEBIAN/control.new

mv $ROOT/deb.sh.new $ROOT/deb.sh
mv $ROOT/macos.sh.new $ROOT/macos.sh
mv $ROOT/windows.ps1.new $ROOT/windows.ps1
mv $ROOT/dist/DEBIAN/control.new $ROOT/dist/DEBIAN/control

if [ "$(git diff --shortstat)" = " 4 files changed, 4 insertions(+), 4 deletions(-)" ] ; then
  echo "Success"
else
    echo "Platform scripts version update failed" 2>&1
    git diff | cat 2>&1
    exit 1
fi
