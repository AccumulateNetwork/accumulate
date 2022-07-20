#!/bin/bash

function printHelp {
    echo "Usage: ";
    echo " -d|--dn <dn working directory>";
    echo " -h|--help show this";
}

function success {
    echo -e '\033[1;32m'[success] "$@"'\033[0m'
    echo
}

function info {
	echo -e '[info]' "$@"
}

function die {
	echo -e '\033[1;31m'[error] "$@"'\033[0m'
	exit 1
}

readonly URI_REGEX='^(([^:/?#]+):)?(//((([^:/?#]+)@)?([^:/?#]+)(:([0-9]+))?))?(/([^?#]*))(\?([^#]*))?(#(.*))?'

function parse_port {
    [[ "$@" =~ $URI_REGEX ]] && echo "${BASH_REMATCH[9]}"
}


workingDir=~/.accumulate
genesis="NO"
network=""
http="http://"
listen=""
dnWorkingDir=""
bvnWorkingDir=""

while [ "$#" -gt 0 ];
do
    case "$1" in
    --help|-h) 
            printHelp && exit 1 ;;
    --dn|-d)
	    dnWorkingDir="$2"
            success "set directory node working directory as ${dnWorkingDir}";
            shift;;
        "") break;;

    esac
    shift
done

[ -z "$dnWorkingDir" ] && printHelp && die "DN working directory not specified"
	
accumulateDnFile=${dnWorkingDir}/config/accumulate.toml
accumulateBvnFile=`realpath ${dnWorkingDir}/../bvnn/config/accumulate.toml`
[ ! -f "$accumulateBvnFile" ] && die "cannot access BVNN Accumulate configuration file `realpath $accumulateBvnFile`"
success "found $accumulateBvnFile"
IFS='=' read token bvnSubnet <<< $(grep partition\-id $accumulateBvnFile)
[ -z "$bvnSubnet" ] && die "partition ID not found in configuration file `realpath $accumulateBvnFile`"
success "identified subnet $bvnSubnet"

accumulateDnKeyFile=${dnWorkingDir}/../priv_validator_key.json
[ ! -f "$accumulateDnKeyFile" ] && die "cannot access DN Accumulate key file $accumulateDnKeyFile"
success "found $accumulateDnKeyFile"

# export the public key and ip address to register validator 

address=$(jq -re .address $accumulateDnKeyFile)
pubkey=$(jq -re .pub_key.value $accumulateDnKeyFile)
pubkey=$(echo $pubkey | base64 -d | od -t x1 -An )
hexPubKey=$(echo $pubkey | tr -d ' ')
regfile=`realpath $dnWorkingDir/../register.json`
echo '{"partition":'$bvnSubnet',"address":"'$address'","pubkey":"'$hexPubKey'"}' |jq > $regfile
echo "Created node validator registration file at $regfile"
cat $regfile

success






