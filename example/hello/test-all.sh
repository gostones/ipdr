#!/bin/bash

# required external services:
# 1) start sshd on 5022 with username/password: app/app
# 2) start ipfs on 5001
set -x

function test_ipdr() {
    IPDR_STORE=$2 ipdr server --port $1 &

    ./test.sh $1 $2
}

##
declare -a arr=(
"file:/tmp/ipdr-file-$RANDOM"
"scp://app:app@local.ipdr.io:5022/tmp/ipdr-scp-$RANDOM"
"ipfs://local.ipdr.io:5001/tmp/ipdr-ipfs-$RANDOM"
)

##
for store in "${arr[@]}"
do
   port=$((5000 + $RANDOM % 1000))
   echo "*** testing $port $store"
   time test_ipdr $port $store
done

exit 0
