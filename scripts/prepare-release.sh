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
cat README.md | sed "s/${LATEST_RELEASE_SEMVER}/${VSN}/" > README.md.new
mv README.md.new README.md