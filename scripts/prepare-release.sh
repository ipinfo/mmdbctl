#!/bin/bash

# Add commits (except merge commits) since current release

set -e

DIR=`dirname $0`
ROOT=$DIR/..

REPO_URL=https://www.github.com/ipinfo/mmdbctl
CHANGES=CHANGELOG.added.md

VSN=$1

if [ -z "${VSN}" ]; then
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

LATEST_RELEASE_VERSION=$(wget -qO- https://api.github.com/repos/ipinfo/mmdbctl/releases/latest | jq -r ".tag_name")
LATEST_RELEASE_SEMVER=$(echo ${LATEST_RELEASE_VERSION} | sed 's/mmdbctl-//' )

# Build list of commits since last release
echo -e "# ${VSN}\n" > $CHANGES
git log ${LATEST_RELEASE_VERSION}..origin/master --oneline --no-merges | while read -r line; do
    COMMIT_HASH=$(echo $line | cut -d' ' -f1)
    COMMIT_MESSAGE=$(echo $line | cut -d' ' -f2-)

    echo "- [${COMMIT_HASH}](${REPO_URL}/commit/${COMMIT_HASH}) ${COMMIT_MESSAGE}" >> $CHANGES
done
echo "" >> $CHANGES

# Overwrite CHANGELOG
cat $CHANGES CHANGELOG.md > CHANGELOG.md.new
mv CHANGELOG.md.new CHANGELOG.md
rm $CHANGES

# Update README with new version
cat README.md | sed "s/${LATEST_RELEASE_SEMVER}/${VSN}/g" > README.md.new
mv README.md.new README.md