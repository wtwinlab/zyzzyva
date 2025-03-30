#!/bin/bash

# # Start a server
# ./zyzzyva -id 0 -type server -batch-size 10

# ./zyzzyva -id 4 -type client -primary 0 -rate 100 -payload-size 64


# ./run_experiment.sh -f 1 -c 3 -t 60 -b 10 -r 100

# ${SCRIPT_DIR}/analyze_results.py experiment_20230715_123456 experiment_20230715_234567



SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

${SCRIPT_DIR}/build.sh

F=1
N=$((3*F+1))  # 3f+1 replicas
CLIENTS=1
RUNTIME=30    # Run for 30 seconds
BATCH_SIZE=1  # Start with no batching for simplicity

# Export environment variables
export F=$F

# Start replica servers
echo "Starting $N replica servers..."
SERVER_PIDS=()
mkdir -p logs

for ((i=0; i<N; i++)); do
    LOG_FILE="logs/server_${i}.log"
    echo "Starting server $i (logging to $LOG_FILE)..."
    
    ${SCRIPT_DIR}/zyzzyva-server -id $i -type server -batch-size $BATCH_SIZE > "$LOG_FILE" 2>&1 &
    SERVER_PIDS+=($!)
    
    echo "  - Server $i started (PID: ${SERVER_PIDS[-1]})"
    sleep 1
done

echo "Waiting for servers to initialize..."
sleep 3

# Start client
echo "Starting client..."
CLIENT_ID=$N
LOG_FILE="logs/client_0.log"

${SCRIPT_DIR}/zyzzyva-client -id $CLIENT_ID -type client -primary 0 -rate 10 -payload-size 64 > "$LOG_FILE" 2>&1 &
CLIENT_PID=$!

echo "  - Client started (PID: $CLIENT_PID)"

# Wait for the test to run
echo "Test running for $RUNTIME seconds..."
sleep $RUNTIME

# Stop all processes
echo "Stopping all processes..."
kill ${SERVER_PIDS[@]} $CLIENT_PID 2>/dev/null || true

# Display results
echo "Test completed. Checking results..."
echo ""
echo "Client output:"
echo "==========================================="
grep -E "Request completed|Fast path|Slow path" "$LOG_FILE" | tail -n 10
echo "==========================================="

# Count requests and successes
TOTAL_REQS=$(grep -c "Sending request" "$LOG_FILE" || echo "0")
SUCCESS_REQS=$(grep -c "Request completed successfully" "$LOG_FILE" || echo "0")

echo ""
echo "Summary:"
echo "  - Total requests: $TOTAL_REQS"
echo "  - Successful requests: $SUCCESS_REQS"

if [ "$TOTAL_REQS" -gt 0 ]; then
    SUCCESS_RATE=$(( (SUCCESS_REQS * 100) / TOTAL_REQS ))
    echo "  - Success rate: $SUCCESS_RATE%"
fi

echo ""
${SCRIPT_DIR}/analyze_results.py ${SCRIPT_DIR}/log


echo "Logs available in the logs directory"