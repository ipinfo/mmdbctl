#!/bin/sh

VSN=1.4.10

case "$(uname -m)" in
    arm64) PLAT=darwin_arm64 ;;
    x86_64)
        if [ "$(sysctl -n hw.optional.arm64) 2>/dev/null" = "1" ]; then
            PLAT=darwin_arm64
        else
            PLAT=darwin_amd64
        fi
        ;;
    *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

curl -LO https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-${VSN}/mmdbctl_${VSN}_${PLAT}.tar.gz
tar -xf mmdbctl_${VSN}_${PLAT}.tar.gz
rm mmdbctl_${VSN}_${PLAT}.tar.gz
mv mmdbctl_${VSN}_${PLAT} /usr/local/bin/mmdbctl

echo
echo 'You can now run `mmdbctl`'.

if [ -f "$0" ]; then
    rm $0
fi
