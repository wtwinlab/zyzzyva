#!/bin/bash
# build.sh - Script to build the Zyzzyva implementation

set -e

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go to build Zyzzyva."
    exit 1
fi

echo "Building Zyzzyva implementation..."

# Set Go module mode if not already set
# export GO111MODULE=on

# # Get dependencies
# go get -v ./...

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

# Build the executable
mkdir -p ${SCRIPT_DIR}/bin
go build -o ${SCRIPT_DIR}/bin/zyzzyva-server ${SCRIPT_DIR}/../cmd/server 
go build -o ${SCRIPT_DIR}/bin/zyzzyva-client ${SCRIPT_DIR}/../cmd/client 

echo "Build complete. The 'zyzzyva' executable has been created."