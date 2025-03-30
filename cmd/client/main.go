package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/myl7/zyzzyva/pkg/client"
	"github.com/myl7/zyzzyva/pkg/conf"
	"github.com/myl7/zyzzyva/pkg/utils"
)

func main() {
	// Client configuration
	primary := flag.Int("primary", 0, "Primary server ID")
	requestRate := flag.Int("rate", conf.RequestRate, "Requests per second (0 = unlimited)")
	requestTimeout := flag.Duration("timeout", conf.ClientTimeout, "Request timeout")
	payloadSize := flag.Int("payload-size", conf.PayloadSize, "Size of request payload in bytes")
	numRequests := flag.Int("requests", 0, "Number of requests to send (0 = unlimited)")
	readOnly := flag.Bool("read-only", false, "Whether requests are read-only")
	payloadPattern := flag.String("payload-pattern", "random", "Payload pattern (random, zeros, ones, incremental)")

	// Basic node configuration
	id := flag.Int("id", -1, "Node ID (required)")
	nodeType := flag.String("type", "", "Node type (server or client)")

	// General configuration
	faults := flag.Int("f", conf.F, "Number of Byzantine faults to tolerate")
	verboseLogging := flag.Bool("verbose", conf.VerboseLogging, "Enable verbose logging")

	flag.Parse()

	if *id < 0 {
		fmt.Println("Error: Node ID is required")
		flag.Usage()
		os.Exit(1)
	}

	// Override configuration from command line
	os.Setenv("F", fmt.Sprintf("%d", *faults))
	// os.Setenv("BATCH_SIZE", fmt.Sprintf("%d", *batchSize))
	// os.Setenv("BATCH_TIMEOUT", batchTimeout.String())
	os.Setenv("REQUEST_RATE", fmt.Sprintf("%d", *requestRate))
	os.Setenv("CLIENT_TIMEOUT", requestTimeout.String())
	os.Setenv("PAYLOAD_SIZE", fmt.Sprintf("%d", *payloadSize))
	os.Setenv("VERBOSE_LOGGING", fmt.Sprintf("%v", *verboseLogging))

	// Initialize configuration
	conf.InitKeys(*id)
	utils.InitLog()
	conf.PrintConfig()

	log.Printf("Node ID %d started as %s", *id, *nodeType)

	log.Printf("ID %d started", *id)

	// Create and run client
	c := client.NewClient(*id, &client.ClientOptions{
		Primary:        *primary,
		RequestRate:    *requestRate,
		ReqTimeout:     *requestTimeout,
		PayloadSize:    *payloadSize,
		NumRequests:    *numRequests,
		ReadOnly:       *readOnly,
		PayloadPattern: *payloadPattern,
	})

	// Run the client
	c.Run()

}
