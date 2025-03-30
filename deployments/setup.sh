#!/bin/bash
# setup.sh - Set up the environment for Zyzzyva experiments

set -e

# Check for required tools
function check_command {
  if ! command -v $1 &> /dev/null; then
    echo "Error: $1 is required but not installed."
    echo "Please install $1 and try again."
    exit 1
  fi
}

echo "Checking for required tools..."
check_command go
check_command python3
check_command pip3

# Install Python dependencies for analysis
echo "Installing Python dependencies for analysis..."
pip3 install matplotlib numpy pandas tabulate --user

# Create directories
echo "Creating necessary directories..."
mkdir -p logs results

# Make sure all scripts are executable
echo "Setting execute permissions on scripts..."
chmod +x build.sh run_experiment.sh analyze_results.py 2>/dev/null || true

# Build the zyzzyva binary
echo "Building zyzzyva binary..."
./build.sh

# Check if keys are available
KEY_FILE="keys.go"
if [ ! -f "$KEY_FILE" ]; then
  echo "Generating key setup file..."
  cat > "$KEY_FILE" << 'EOF'
package conf

import (
	"crypto/rand"
	"crypto/rsa"
	"log"
)

// GenerateKeys generates RSA key pairs for testing
func GenerateKeys() {
	// Only generate keys if not already initialized
	if Pub == nil || Priv == nil {
		log.Println("Generating keys for testing...")
		
		totalNodes := N + M
		Pub = make([]*rsa.PublicKey, totalNodes)
		Priv = make([]*rsa.PrivateKey, totalNodes)
		
		for i := 0; i < totalNodes; i++ {
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				panic(err)
			}
			Priv[i] = key
			Pub[i] = &key.PublicKey
		}
		
		log.Printf("Generated %d key pairs", totalNodes)
	}
}

// InitKeys initializes keys for the specified node ID
func InitKeys(id int) {
	// Generate all keys locally for testing
	GenerateKeys()
}
EOF

  # Move the key file to the appropriate location
  mkdir -p pkg/conf
  mv "$KEY_FILE" pkg/conf/
  
  echo "Created key setup file for local testing"
fi

echo "Setup complete! You can now run experiments with ./run_experiment.sh"