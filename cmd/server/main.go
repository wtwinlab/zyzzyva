package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/myl7/zyzzyva/pkg/conf"
	"github.com/myl7/zyzzyva/pkg/server"
	"github.com/myl7/zyzzyva/pkg/utils"
)

func main() {
	// Basic node configuration
	id := flag.Int("id", -1, "Node ID (required)")
	nodeType := flag.String("type", "", "Node type (server or client)")

	// Server configuration
	batchSize := flag.Int("batch-size", conf.BatchSize, "Number of requests to batch")
	batchTimeout := flag.Duration("batch-timeout", conf.BatchTimeout, "Maximum time to wait for a batch")

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
	os.Setenv("BATCH_SIZE", fmt.Sprintf("%d", *batchSize))
	os.Setenv("BATCH_TIMEOUT", batchTimeout.String())
	// os.Setenv("REQUEST_RATE", fmt.Sprintf("%d", *requestRate))
	// os.Setenv("CLIENT_TIMEOUT", requestTimeout.String())
	// os.Setenv("PAYLOAD_SIZE", fmt.Sprintf("%d", *payloadSize))
	os.Setenv("VERBOSE_LOGGING", fmt.Sprintf("%v", *verboseLogging))

	conf.InitKeys(*id)
	utils.InitLog()
	conf.PrintConfig()

	log.Printf("Node ID %d started as %s", *id, *nodeType)

	// Setup signal handler for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	s := server.NewServer(*id, &server.ServerOptions{
		BatchSize:    *batchSize,
		BatchTimeout: *batchTimeout,
		StartView:    0,
	})

	go s.Run()

	// Wait for termination signal
	<-sigChan
	log.Println("Received termination signal, shutting down...")

	s.PrintStats()

}
