#!/bin/sh

VSN=1.4.7

curl -LO https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-${VSN}/mmdbctl_${VSN}.deb
sudo dpkg -i mmdbctl_${VSN}.deb
rm mmdbctl_${VSN}.deb

echo
echo 'You can now run `mmdbctl`'.

if [ -f "$0" ]; then
    rm $0
fi
