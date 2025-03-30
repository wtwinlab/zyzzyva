#!/bin/bash
# run_experiment.sh - Run a Zyzzyva experiment with specified parameters

set -x

# Default configuration
F=${1:-1}
N=$((3*F+1))
CLIENTS=${2:-1}
REQUEST_RATE=${3:-100}
BATCH_SIZE=${4:-10}
PAYLOAD_SIZE=${5:-64}
RUNTIME=${6:-60}
IP_PREFIX="127.0.0.1"

pkill zyzzyva-server
pkill zyzzyva-client
rm -rf ./logs/
rm -rf ./experiment_results
mkdir -p logs
mkdir -p experiment_results

# Start 4 replica servers (for f=1)
for ((i=0; i<$N; i++)); do
    ./bin/zyzzyva-server -id $i -type server -batch-size 10 > logs/server_${i}.log 2>&1 &
    SERVER_PIDS+=($!)
done

# Give servers time to initialize
sleep 3

# Start client
./bin/zyzzyva-client -id $N -type client -primary 0 -rate 100 -payload-size 64 > logs/client_${N}.log 2>&1 &
CLIENT_PID=$!

# Run for 60 seconds
sleep 60

mv client_*_stats.json ./experiment_results/
python3 analyze_results.py experiment_results/
# Stop all processes
kill ${SERVER_PIDS[@]} $CLIENT_PID 2>/dev/null || true