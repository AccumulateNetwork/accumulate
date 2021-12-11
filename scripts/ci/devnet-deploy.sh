#!/bin/bash

if [ -z "${BIN}" ] || [ -z "${HOST}" ] || [ -z "${NETWORK}" ] || [ -z "${NODE}" ] || [ -z "${DN_NODE}" ]; then
    echo "Environment variables must be specified"
    echo "  HOST: '${HOST}'"
    echo "  NETWORK: '${NETWORK}'"
    echo "  NODE: '${NODE}'"
    echo "  DN_NODE: '${DN_NODE}'"
    echo "  BIN: '${BIN}'"
    exit 1
fi

# Deploy configuration and the binary
scp ${BIN} ec2-user@${HOST}:accumulated-latest
scp config-DevNet.Directory.tar.gz ec2-user@${HOST}:config-dn.tar.gz
scp config-DevNet.${NETWORK}.tar.gz ec2-user@${HOST}:config-bvn.tar.gz
ssh ec2-user@${HOST} '~/accumulated-latest version'

# Check if deploy is enabled
echo ${DEPLOY} | grep -q DevNet.${NETWORK} || exit 0

ssh ec2-user@${HOST} "
    set -ex
    screen -wipe || true
    screen -ls | grep '\baccumulated\s' && screen -S accumulated -X stuff ^C
    screen -ls | grep '\baccumulated-bvn\s' && screen -S accumulated-bvn -X stuff ^C
    screen -ls | grep '\baccumulated-dn\s' && screen -S accumulated-dn -X stuff ^C
    while screen -ls | grep -Eq '\baccumulated'; do sleep 1; done
    rm -rf ~/.local/bin/accumulated ~/.accumulate/{bvn,dn,Node*}
    mkdir -p ~/.local/bin ~/.accumulate/{bvn,dn,logs}
    mv accumulated-latest ~/.local/bin/accumulated
    tar xCf ~/.accumulate/bvn config-bvn.tar.gz Node${NODE} --strip-components 1
    tar xCf ~/.accumulate/dn config-dn.tar.gz Node${DN_NODE} --strip-components 1
    export PATH="'"$PATH:$HOME/.local/bin"'"
    screen -d -m -S accumulated-dn bash -c 'accumulated run -w ~/.accumulate/dn 2>&1 | tee ~/.accumulate/logs/acc-dn-$(date +%Y%m%d%H%M%S).log'
    screen -d -m -S accumulated-bvn bash -c 'accumulated run -w ~/.accumulate/bvn 2>&1 | tee ~/.accumulate/logs/acc-bvn-$(date +%Y%m%d%H%M%S).log'
"