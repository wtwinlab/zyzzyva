#!/bin/bash
# run_experiment.sh - Run a Zyzzyva experiment with specified parameters

set -e

# Default configuration
F=${F:-1}
N=$((3*F+1))
CLIENTS=${CLIENTS:-1}
RUNTIME=${RUNTIME:-60}
BATCH_SIZE=${BATCH_SIZE:-10}
REQUEST_RATE=${REQUEST_RATE:-100}
PAYLOAD_SIZE=${PAYLOAD_SIZE:-64}
IP_PREFIX=${IP_PREFIX:-"127.0.0.1"}
VERBOSE=${VERBOSE:-false}

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
        -f|--faults)
            F="$2"
            N=$((3*F+1))
            shift 2
            ;;
        -c|--clients)
            CLIENTS="$2"
            shift 2
            ;;
        -t|--time)
            RUNTIME="$2"
            shift 2
            ;;
        -b|--batch)
            BATCH_SIZE="$2"
            shift 2
            ;;
        -r|--rate)
            REQUEST_RATE="$2"
            shift 2
            ;;
        -s|--size)
            PAYLOAD_SIZE="$2"
            shift 2
            ;;
        -i|--ip)
            IP_PREFIX="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  -f, --faults F       Number of Byzantine faults to tolerate (default: $F)"
            echo "  -c, --clients N      Number of clients (default: $CLIENTS)"
            echo "  -t, --time SEC       Runtime in seconds (default: $RUNTIME)"
            echo "  -b, --batch SIZE     Batch size (default: $BATCH_SIZE)"
            echo "  -