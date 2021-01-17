#!/bin/bash

set -x
set -e
set -o pipefail

#
export IPDR_PORT=${1:-"5000"}
export IPDR_STORE=${2:-""}

export DOCKER_REGISTRY_HOST="local.ipdr.io:$IPDR_PORT"
export IPFS_PATH=${IPFS_PATH:-"$HOME/.ipdr/ipfs/data"}

# build

if ! command -v ipdr &> /dev/null
then
    echo "  *** ipdr not found, building it..."
    (cd ../../../ipdr && go install ./cmd/ipdr)
fi

#
function build_run {
    # build Docker image
    docker build --quiet -t $1 --build-arg REPO_NAME=$1 --build-arg DOCKER_REGISTRY_HOST=$DOCKER_REGISTRY_HOST .

    # test run
    docker run $1
}

function cleanup {
    docker rmi -f $(docker image ls -q $1)
}

#
my=$DOCKER_REGISTRY_HOST

###
repo_name="hello/ipdr-cli:v0.0.1-b$RANDOM"
echo "  *** push/pull $repo_name using ipdr cli..."
build_run $repo_name

# push to IPFS
IPFS_HASH="$(ipdr push $repo_name --silent --docker-registry-host=$DOCKER_REGISTRY_HOST)"

# pull from IPFS
REPO_TAG=$(ipdr pull "$IPFS_HASH" --silent --docker-registry-host=$DOCKER_REGISTRY_HOST)

# run image pulled from IPFS
docker run "$REPO_TAG"

# clean up
cleanup $repo_name

###
repo_name="hello/docker-cli:v0.0.1-b$RANDOM"
echo "  *** push/pull $repo_name using docker cli..."
build_run $repo_name

# push to IPFS
docker tag $repo_name $my/$repo_name
docker push --quiet $my/$repo_name

# pull from IPFS
docker pull --quiet $my/$repo_name

# run image pulled from IPFS
docker run $my/$repo_name

# clean up
cleanup $repo_name

###
echo "  compatibility tests..."

###
repo_name="hello/ipdr/docker:v0.0.1-b$RANDOM"
echo "  *** ipdr push/docker pull $repo_name..."
build_run $repo_name

# push to IPFS
IPFS_HASH="$(ipdr push $repo_name --silent --docker-registry-host=$DOCKER_REGISTRY_HOST)"

# pull from IPFS
docker pull --quiet $my/$IPFS_HASH


# run image pulled from IPFS
docker run $my/$IPFS_HASH

# clean up
cleanup $repo_name

###
repo_name="hello/docker/ipdr:v0.0.1-b$RANDOM"
echo "  *** docker push/ipdr pull $repo_name..."
build_run $repo_name

# push to IPFS
docker tag $repo_name $my/$repo_name
docker push --quiet $my/$repo_name

# pull from IPFS
IPFS_HASH=$(ipdr dig $repo_name --short=true --docker-registry-host=$DOCKER_REGISTRY_HOST)
REPO_TAG=$(ipdr pull "$IPFS_HASH" --silent --docker-registry-host=$DOCKER_REGISTRY_HOST)

# run image pulled from IPFS
docker run "$REPO_TAG"

# clean up
cleanup $repo_name
echo "test complete."
