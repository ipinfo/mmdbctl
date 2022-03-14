#!/bin/sh

VSN=1.2.0
PLAT=darwin_amd64

curl -LO https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-${VSN}/mmdbctl_${VSN}_${PLAT}.tar.gz
tar -xf mmdbctl_${VSN}_${PLAT}.tar.gz
rm mmdbctl_${VSN}_${PLAT}.tar.gz
mv mmdbctl_${VSN}_${PLAT} /usr/local/bin/mmdbctl

echo
echo 'You can now run `mmdbctl`'.

if [ -f "$0" ]; then
    rm $0
fi
