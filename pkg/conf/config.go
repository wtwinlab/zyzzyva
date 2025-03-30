package conf

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Configuration constants that can be overridden with environment variables
var (
	// Fault tolerance parameters
	F = envInt("F", 1)     // Number of faults to tolerate
	N = envInt("N", 3*F+1) // Total number of replicas (default: 3f+1)
	M = envInt("M", 1)     // Number of clients (can be overridden)

	// Consensus parameters
	BatchSize    = envInt("BATCH_SIZE", 1)                            // Number of requests to batch together
	BatchTimeout = envDuration("BATCH_TIMEOUT", 100*time.Millisecond) // Max time to collect a batch

	// Client parameters
	ClientTimeout = envDuration("CLIENT_TIMEOUT", 10*time.Second) // Timeout for client requests
	RequestRate   = envInt("REQUEST_RATE", 0)                     // Requests per second (0 = unlimited)
	PayloadSize   = envInt("PAYLOAD_SIZE", 64)                    // Default request payload size

	// Checkpointing parameters
	CPInterval = envInt("CP_INTERVAL", 100) // Checkpoint interval (# of requests)

	// Protocol settings
	UseFastRead = envBool("USE_FAST_READ", true) // Whether to use fast-path for read-only
	UseMAC      = envBool("USE_MAC", false)      // Whether to use MACs instead of signatures (not implemented)

	// Network settings
	UdpBufSize             = envInt("UDP_BUF_SIZE", 1*1024*1024)                        // UDP buffer size
	IpPrefix               = envStr("IP_PREFIX", "127.0.0.1")                           // IP prefix for network
	BasePort               = envInt("BASE_PORT", 10000)                                 // Base port for communication
	UdpMulticastAddr       = envStr("UDP_MULTICAST_ADDR", "239.255.0.1:10001")          // Multicast address
	UdpMulticastInterfaces = strings.Split(envStr("UDP_MULTICAST_INTERFACES", ""), ",") // Network interfaces for multicast

	// Debugging and logging
	VerboseLogging        = envBool("VERBOSE_LOGGING", false) // Enable verbose logging
	ExtraNonDeterministic = []byte("extra")                   // Non-deterministic values
	Extra                 = []byte("extra")                   // Backwards compatibility
)

// Helper functions to read environment variables with defaults

func envStr(key string, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if val, err := strconv.Atoi(v); err == nil {
			return val
		}
		fmt.Printf("Warning: Environment variable %s is not a valid integer, using default: %d\n", key, defaultVal)
	}
	return defaultVal
}

func envBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		if strings.ToLower(v) == "true" || v == "1" {
			return true
		} else if strings.ToLower(v) == "false" || v == "0" {
			return false
		}
		fmt.Printf("Warning: Environment variable %s is not a valid boolean, using default: %v\n", key, defaultVal)
	}
	return defaultVal
}

func envDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if duration, err := time.ParseDuration(v); err == nil {
			return duration
		}
		fmt.Printf("Warning: Environment variable %s is not a valid duration, using default: %v\n", key, defaultVal)
	}
	return defaultVal
}

// Network configuration functions

func GetReqAddr(id int) string {
	i := strings.LastIndex(IpPrefix, ".")
	if i == -1 {
		panic(errors.New("invalid ip prefix"))
	}

	last, err := strconv.Atoi(IpPrefix[i+1:])
	if err != nil {
		panic(err)
	}

	return fmt.Sprintf("%s.%d:%d", IpPrefix[:i], id+last, BasePort)
}

func GetListenAddr(id int) string {
	return fmt.Sprintf(":%d", BasePort)
}

// Other utility functions

func GetRandInput(b []byte) ([]byte, error) {
	if b == nil {
		b = make([]byte, PayloadSize)
	}
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// PrintConfig outputs the current configuration to the log
func PrintConfig() {
	fmt.Println("=== Zyzzyva Configuration ===")
	fmt.Printf("F (Byzantine Faults): %d\n", F)
	fmt.Printf("N (Total Replicas): %d\n", N)
	fmt.Printf("M (Clients): %d\n", M)
	fmt.Printf("Batch Size: %d\n", BatchSize)
	fmt.Printf("Batch Timeout: %v\n", BatchTimeout)
	fmt.Printf("Client Timeout: %v\n", ClientTimeout)
	fmt.Printf("Checkpoint Interval: %d\n", CPInterval)
	fmt.Printf("Network: %s.x:%d\n", IpPrefix, BasePort)
	fmt.Printf("Multicast: %s\n", UdpMulticastAddr)
	if len(UdpMulticastInterfaces) > 0 && UdpMulticastInterfaces[0] != "" {
		fmt.Printf("Multicast Interfaces: %s\n", strings.Join(UdpMulticastInterfaces, ", "))
	}
	fmt.Println("===========================")
}
