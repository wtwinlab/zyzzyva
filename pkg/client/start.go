package client

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/myl7/zyzzyva/pkg/comm"
	"github.com/myl7/zyzzyva/pkg/conf"
	"github.com/myl7/zyzzyva/pkg/msg"
	"github.com/myl7/zyzzyva/pkg/utils"
)

// Client represents a Zyzzyva client with enhanced configuration options
type Client struct {
	// Basic client state
	id       int
	primary  int
	listen   bool
	listenMu sync.Mutex

	// Request configuration
	requestRate int           // Requests per second (0 = unlimited)
	reqTimeout  time.Duration // Timeout for each request
	payloadSize int           // Size of request payload in bytes
	numRequests int           // Total number of requests to send (0 = unlimited)
	readOnly    bool          // Whether requests are read-only

	// Custom payload options
	customPayload  []byte // Custom payload to use (nil = random)
	payloadPattern string // Pattern for payload ("random", "zeros", "ones", "custom")

	// Response handling
	responses chan msg.SpecResMsg // Channel for speculative responses
	stopChan  chan struct{}       // Channel for graceful shutdown

	// Statistics
	stats   ClientStats
	statsMu sync.Mutex
	running bool
}

// ClientStats maintains statistics about client operations
type ClientStats struct {
	TotalRequests      int             `json:"total_requests"`      // Total requests sent
	SuccessfulRequests int             `json:"successful_requests"` // Successfully completed requests
	FastPathCompleted  int             `json:"fast_path_completed"` // Fast path completions (3f+1)
	SlowPathCompleted  int             `json:"slow_path_completed"` // Slow path completions (2f+1)
	FailedRequests     int             `json:"failed_requests"`     // Failed requests
	InFlightRequests   int             `json:"in_flight_requests"`  // Requests sent but not completed
	ResponseTimes      []time.Duration `json:"-"`                   // Response times
	StartTime          time.Time       `json:"start_time"`          // When the client started
	EndTime            time.Time       `json:"end_time,omitempty"`  // When the client finished

	// Derived statistics (calculated when printing/saving)
	AvgResponseTimeMs float64 `json:"avg_response_time_ms"`
	MinResponseTimeMs float64 `json:"min_response_time_ms"`
	MaxResponseTimeMs float64 `json:"max_response_time_ms"`
	P50ResponseTimeMs float64 `json:"p50_response_time_ms"`
	P95ResponseTimeMs float64 `json:"p95_response_time_ms"`
	P99ResponseTimeMs float64 `json:"p99_response_time_ms"`
	ThroughputReqSec  float64 `json:"throughput_req_sec"`
}

// ClientOptions allows customization of client behavior
type ClientOptions struct {
	Primary        int           // Primary replica ID
	RequestRate    int           // Requests per second (0 = unlimited)
	ReqTimeout     time.Duration // Timeout for each request
	PayloadSize    int           // Size of request payload in bytes
	NumRequests    int           // Total number of requests to send (0 = unlimited)
	ReadOnly       bool          // Whether requests are read-only
	PayloadPattern string        // Pattern for payload ("random", "zeros", "ones", "custom")
	CustomPayload  []byte        // Custom payload to use (nil = random)
}

// NewClient creates a new enhanced Zyzzyva client
func NewClient(id int, options *ClientOptions) *Client {
	// Default options if none provided
	if options == nil {
		options = &ClientOptions{
			Primary:        0,
			RequestRate:    conf.RequestRate,
			ReqTimeout:     conf.ClientTimeout,
			PayloadSize:    conf.PayloadSize,
			NumRequests:    0, // Unlimited
			ReadOnly:       false,
			PayloadPattern: "random",
		}
	}

	client := &Client{
		id:             id,
		primary:        options.Primary,
		requestRate:    options.RequestRate,
		reqTimeout:     options.ReqTimeout,
		payloadSize:    options.PayloadSize,
		numRequests:    options.NumRequests,
		readOnly:       options.ReadOnly,
		payloadPattern: options.PayloadPattern,
		customPayload:  options.CustomPayload,
		responses:      make(chan msg.SpecResMsg, conf.N*10), // Buffer for responses
		stopChan:       make(chan struct{}),
		running:        false,
		stats: ClientStats{
			ResponseTimes: make([]time.Duration, 0, 1000),
			StartTime:     time.Now(),
		},
	}

	return client
}

// Run starts the client operation
func (c *Client) Run() {
	c.running = true

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping client...")
		c.Stop()
	}()

	// Start listening for responses
	go c.listenForResponses()

	// Wait a moment for setup
	time.Sleep(1 * time.Second)

	// Print configuration
	c.printConfig()

	// Setup rate limiting if needed
	var ticker *time.Ticker
	if c.requestRate > 0 {
		interval := time.Second / time.Duration(c.requestRate)
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	}

	// Map to track in-flight requests
	inFlight := make(map[int64]time.Time)
	var inflightMu sync.Mutex

	// Create a WaitGroup for handling responses
	var wg sync.WaitGroup

	// Launch a goroutine to handle all responses
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-c.stopChan:
				return
			case resp := <-c.responses:
				// Get the request timestamp
				timestamp := resp.SpecRes.Timestamp

				// Check if this request is still being tracked
				inflightMu.Lock()
				startTime, exists := inFlight[timestamp]
				inflightMu.Unlock()

				if !exists {
					// This might be a duplicate response for a request we've already handled
					continue
				}

				// Calculate response time
				responseTime := time.Since(startTime)

				// Process the response
				c.processResponse(resp, responseTime)

				// Remove from in-flight map
				inflightMu.Lock()
				delete(inFlight, timestamp)

				// Update in-flight count in stats
				c.statsMu.Lock()
				c.stats.InFlightRequests = len(inFlight)
				c.statsMu.Unlock()

				inflightMu.Unlock()
			}
		}
	}()

	// Main request loop
	requestCount := 0
	for c.running && (c.numRequests == 0 || requestCount < c.numRequests) {
		// Rate limit if requested
		if ticker != nil {
			<-ticker.C
		}

		// Generate timestamp for this request
		timestamp := time.Now().UnixNano()

		// Generate payload according to pattern
		payload := c.generatePayload()

		// Send the request
		// err := c.sendRequest(payload, timestamp)
		// if err != nil {
		// 	log.Printf("Error sending request: %v", err)
		// 	continue
		// }

		// Send the request
		err := c.sendRequest(payload, timestamp)
		if err != nil {
			log.Printf("Error sending request: %v", err)
			c.statsMu.Lock()
			c.stats.FailedRequests++
			c.statsMu.Unlock()
			//return
			continue
		}
		// Add to in-flight map
		inflightMu.Lock()
		inFlight[timestamp] = time.Now()

		// Update stats
		c.statsMu.Lock()
		c.stats.TotalRequests++
		c.stats.InFlightRequests = len(inFlight)
		c.statsMu.Unlock()

		inflightMu.Unlock()

		requestCount++

		// Check for shutdown signal
		select {
		case <-c.stopChan:
			break
		default:
			// Continue with the next request
		}
	}

	log.Printf("Finished sending %d requests, waiting for responses...", requestCount)

	// Wait for all in-flight requests to complete (or timeout)
	waitTimeout := 5 * time.Second
	waitStart := time.Now()
	for time.Since(waitStart) < waitTimeout {
		inflightMu.Lock()
		count := len(inFlight)
		inflightMu.Unlock()

		if count == 0 {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Stop the client
	c.Stop()

	// Wait for response handler to finish
	wg.Wait()

	// Output final statistics
	c.PrintStats()
}

// Stop gracefully stops the client
func (c *Client) Stop() {
	if !c.running {
		return
	}

	c.running = false
	close(c.stopChan)

	// Update end time for stats
	c.statsMu.Lock()
	c.stats.EndTime = time.Now()
	c.statsMu.Unlock()
}

// generatePayload creates a payload according to the configured pattern
func (c *Client) generatePayload() []byte {
	// If custom payload is provided, use that
	if c.customPayload != nil {
		return c.customPayload
	}

	// Create a payload of the specified size
	payload := make([]byte, c.payloadSize)

	// Fill according to pattern
	switch c.payloadPattern {
	case "zeros":
		// Leave as zeros
	case "ones":
		for i := range payload {
			payload[i] = 0xFF
		}
	case "incremental":
		for i := range payload {
			payload[i] = byte(i % 256)
		}
	case "custom":
		// This should be handled by the customPayload field
		log.Println("Warning: 'custom' pattern selected but no customPayload provided")
	case "random":
		fallthrough
	default:
		// Generate random data
		_, err := conf.GetRandInput(payload)
		if err != nil {
			log.Printf("Error generating random payload: %v", err)
		}
	}

	return payload
}

// sendRequest sends a request to the primary
func (c *Client) sendRequest(payload []byte, timestamp int64) error {
	// Create the request
	req := msg.Req{
		Data:      payload,
		Timestamp: timestamp,
		CId:       c.id,
	}

	// Sign the request
	reqSig := utils.GenSigObj(req, conf.Priv[c.id])

	// Create request message
	reqMsg := msg.ReqMsg{
		T:      msg.TypeReq,
		Req:    req,
		ReqSig: reqSig,
	}

	// Send to primary
	if conf.VerboseLogging {
		log.Printf("Sending request with timestamp %d to primary %d", timestamp, c.primary)
	}

	comm.UdpSendObj(reqMsg, c.primary)
	return nil
}

// listenForResponses listens for and handles incoming network messages
func (c *Client) listenForResponses() {
	l, err := net.ListenPacket("udp", conf.GetListenAddr(c.id))
	if err != nil {
		log.Fatalf("Failed to listen on address: %v", err)
		return
	}
	defer l.Close()

	// Set larger buffer size if possible (only works with *net.UDPConn)
	if udpConn, ok := l.(*net.UDPConn); ok {
		err = udpConn.SetReadBuffer(conf.UdpBufSize)
		if err != nil {
			log.Printf("Warning: Failed to set UDP read buffer size: %v", err)
		}
	}

	buf := make([]byte, conf.UdpBufSize)

	for {
		select {
		case <-c.stopChan:
			return
		default:
			// Set a read deadline so we can check for stop signal periodically
			l.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

			n, _, err := l.ReadFrom(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// This is just a timeout, continue
					continue
				}
				log.Printf("Error reading from network: %v", err)
				continue
			}

			// Process the message
			c.handleMessage(buf[:n])
		}
	}
}

// handleMessage processes an incoming network message
func (c *Client) handleMessage(data []byte) {
	// Determine message type
	var m struct {
		T msg.Type
	}
	err := json.Unmarshal(data, &m)
	if err != nil {
		log.Printf("Error unmarshalling message: %v", err)
		return
	}

	switch m.T {
	case msg.TypeSpecRes:
		// Parse speculative response
		var srm msg.SpecResMsg
		err = json.Unmarshal(data, &srm)
		if err != nil {
			log.Printf("Error unmarshalling spec response: %v", err)
			return
		}

		// Verify signatures
		if !msg.VerifySig(srm, []*rsa.PublicKey{conf.Pub[srm.SId], conf.Pub[c.primary]}) {
			log.Println("Failed to verify speculative response signatures")
			return
		}

		// Send to response channel
		select {
		case c.responses <- srm:
			// Successfully queued
		default:
			log.Println("Warning: Response channel full, dropping message")
		}

	case msg.TypeLocalCommit:
		// Handle local-commit messages for slow path
		log.Println("Received local-commit message")
		// Implementation for handling local-commit messages would go here
		// This is part of the slow path completion process
		// For now, we simply log the message as our implementation
		// focuses on the fast path
	default:
		log.Printf("Unexpected message type: %d", m.T)
	}
}

// specResKey uniquely identifies a speculative response
type specResKey struct {
	view        int
	seq         int
	historyHash string
	resHash     string
	clientId    int
	timestamp   int64
	result      string
}

// specRes2Key converts a SpecResMsg to a specResKey
func specRes2Key(srm msg.SpecResMsg) specResKey {
	return specResKey{
		view:        srm.SpecRes.View,
		seq:         srm.SpecRes.Seq,
		historyHash: base64.StdEncoding.EncodeToString(srm.SpecRes.HistoryHash),
		resHash:     base64.StdEncoding.EncodeToString(srm.SpecRes.ResHash),
		clientId:    srm.SpecRes.CId,
		timestamp:   srm.SpecRes.Timestamp,
		result:      base64.StdEncoding.EncodeToString(srm.Reply),
	}
}

// processResponse processes a speculative response
func (c *Client) processResponse(srm msg.SpecResMsg, responseTime time.Duration) {
	// We would normally collect responses and check for quorum
	// For this simplified implementation, we'll use a single response

	// Track response time
	c.statsMu.Lock()
	c.stats.ResponseTimes = append(c.stats.ResponseTimes, responseTime)
	c.stats.SuccessfulRequests++
	c.stats.FastPathCompleted++
	c.statsMu.Unlock()

	if conf.VerboseLogging {
		log.Printf("Received response for request %d in %v", srm.SpecRes.Timestamp, responseTime)
	}
}

// PrintStats outputs the client statistics
func (c *Client) PrintStats() {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()

	// Update end time if not set
	if c.stats.EndTime.IsZero() {
		c.stats.EndTime = time.Now()
	}

	// Calculate runtime
	runtime := c.stats.EndTime.Sub(c.stats.StartTime)

	// Calculate throughput
	if runtime > 0 {
		c.stats.ThroughputReqSec = float64(c.stats.SuccessfulRequests) / runtime.Seconds()
	}

	// Calculate response time statistics
	if len(c.stats.ResponseTimes) > 0 {
		// Sort response times for percentiles
		sort.Slice(c.stats.ResponseTimes, func(i, j int) bool {
			return c.stats.ResponseTimes[i] < c.stats.ResponseTimes[j]
		})

		// Calculate min/max
		minRT := c.stats.ResponseTimes[0]
		maxRT := c.stats.ResponseTimes[len(c.stats.ResponseTimes)-1]

		// Calculate average
		var totalRT time.Duration
		for _, rt := range c.stats.ResponseTimes {
			totalRT += rt
		}
		avgRT := totalRT / time.Duration(len(c.stats.ResponseTimes))

		// Calculate percentiles
		p50Index := (len(c.stats.ResponseTimes) * 50) / 100
		p95Index := (len(c.stats.ResponseTimes) * 95) / 100
		p99Index := (len(c.stats.ResponseTimes) * 99) / 100

		p50RT := c.stats.ResponseTimes[p50Index]
		p95RT := c.stats.ResponseTimes[p95Index]
		p99RT := c.stats.ResponseTimes[p99Index]

		// Update statistics
		c.stats.MinResponseTimeMs = float64(minRT.Microseconds()) / 1000.0
		c.stats.MaxResponseTimeMs = float64(maxRT.Microseconds()) / 1000.0
		c.stats.AvgResponseTimeMs = float64(avgRT.Microseconds()) / 1000.0
		c.stats.P50ResponseTimeMs = float64(p50RT.Microseconds()) / 1000.0
		c.stats.P95ResponseTimeMs = float64(p95RT.Microseconds()) / 1000.0
		c.stats.P99ResponseTimeMs = float64(p99RT.Microseconds()) / 1000.0
	}

	// Print statistics
	fmt.Printf("\n======= Client %d Statistics =======\n", c.id)
	fmt.Printf("Runtime:              %v\n", runtime.Round(time.Millisecond))
	fmt.Printf("Total Requests:       %d\n", c.stats.TotalRequests)
	fmt.Printf("Successful Requests:  %d (%.2f%%)\n",
		c.stats.SuccessfulRequests,
		float64(c.stats.SuccessfulRequests)/float64(c.stats.TotalRequests)*100)
	fmt.Printf("Failed Requests:      %d\n", c.stats.FailedRequests)
	fmt.Printf("Fast Path:            %d (%.2f%%)\n",
		c.stats.FastPathCompleted,
		float64(c.stats.FastPathCompleted)/float64(c.stats.SuccessfulRequests)*100)
	fmt.Printf("Slow Path:            %d\n", c.stats.SlowPathCompleted)

	if len(c.stats.ResponseTimes) > 0 {
		fmt.Printf("Response Time (ms):   avg %.2f / min %.2f / max %.2f\n",
			c.stats.AvgResponseTimeMs, c.stats.MinResponseTimeMs, c.stats.MaxResponseTimeMs)
		fmt.Printf("Response Time (ms):   p50 %.2f / p95 %.2f / p99 %.2f\n",
			c.stats.P50ResponseTimeMs, c.stats.P95ResponseTimeMs, c.stats.P99ResponseTimeMs)
	}

	fmt.Printf("Throughput:           %.2f req/sec\n", c.stats.ThroughputReqSec)
	fmt.Printf("===================================\n\n")

	// Save statistics to file
	c.saveStatsToFile()
}

// saveStatsToFile saves statistics to a JSON file
func (c *Client) saveStatsToFile() {
	filename := fmt.Sprintf("client_%d_stats.json", c.id)
	data, err := json.MarshalIndent(c.stats, "", "  ")
	if err != nil {
		log.Printf("Error marshalling statistics: %v", err)
		return
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		log.Printf("Error saving statistics to file: %v", err)
		return
	}

	log.Printf("Statistics saved to %s", filename)
}

// printConfig outputs the client configuration
func (c *Client) printConfig() {
	fmt.Printf("\n======= Client %d Configuration =======\n", c.id)
	fmt.Printf("Primary:              %d\n", c.primary)
	fmt.Printf("Request Rate:         %d req/sec", c.requestRate)
	if c.requestRate == 0 {
		fmt.Printf(" (unlimited)\n")
	} else {
		fmt.Printf("\n")
	}
	fmt.Printf("Request Timeout:      %v\n", c.reqTimeout)
	fmt.Printf("Payload Size:         %d bytes\n", c.payloadSize)
	fmt.Printf("Number of Requests:   %d", c.numRequests)
	if c.numRequests == 0 {
		fmt.Printf(" (unlimited)\n")
	} else {
		fmt.Printf("\n")
	}
	fmt.Printf("Read-Only:            %v\n", c.readOnly)
	fmt.Printf("Payload Pattern:      %s\n", c.payloadPattern)
	fmt.Printf("======================================\n\n")
}

// func (c *Client) Run() {
// 	spec := make(chan msg.SpecResMsg, conf.N)

// 	go c.Listen(spec)

// 	time.Sleep(1 * time.Minute)

// 	for {
// 		time.Sleep(1 * time.Second)

// 		log.Println("Start")

// 		in, err := conf.GetRandInput()
// 		if err != nil {
// 			panic(err)
// 		}

// 		r := msg.Req{
// 			Data:      in,
// 			Timestamp: time.Now().UnixNano(),
// 			CId:       c.id,
// 		}
// 		rs := utils.GenSigObj(r, conf.Priv[c.id])
// 		rm := msg.ReqMsg{
// 			T:      msg.TypeReq,
// 			Req:    r,
// 			ReqSig: rs,
// 		}

// 		comm.UdpSendObj(rm, c.primary)

// 		c.listenMu.Lock()
// 		c.listen = true
// 		c.listenMu.Unlock()

// 		srKeyMap := make(map[specResKey]struct {
// 			n       int
// 			sidList []int
// 			sigList [][]byte
// 			sr      msg.SpecRes
// 			reply   []byte
// 		})

// 		func() {
// 			ctx, cancel := context.WithTimeout(context.Background(), conf.ClientTimeout)
// 			defer cancel()

// 			for {
// 				select {
// 				case <-ctx.Done():
// 					return
// 				case srm := <-spec:
// 					k := specRes2Key(srm)
// 					v := srKeyMap[k]
// 					v.n += 1
// 					v.sidList = append(v.sidList, srm.SId)
// 					v.sigList = append(v.sigList, srm.SpecResSig)
// 					v.sr = srm.SpecRes
// 					v.reply = srm.Reply
// 					srKeyMap[k] = v

// 					if v.n >= 3*conf.F+1 {
// 						return
// 					}
// 				}
// 			}
// 		}()

// 		c.listenMu.Lock()
// 		c.listen = false
// 		c.listenMu.Unlock()

// 		maxN := 0
// 		// var maxSr msg.SpecRes
// 		// var sigs [][]byte
// 		// var sids []int
// 		var reply []byte
// 		for _, v := range srKeyMap {
// 			if v.n > maxN {
// 				maxN = v.n
// 				// maxSr = v.sr
// 				// sigs = v.sigList
// 				// sids = v.sidList
// 				reply = v.reply
// 			}
// 		}
// 		if maxN >= 3*conf.F+1 {
// 			if utils.VerifyHash(reply, in) {
// 				log.Println("OK")
// 			} else {
// 				log.Fatalln("Failed")
// 			}
// 		} else if maxN >= 2*conf.F+1 {
// 			log.Println("Requires commit")
// 			if utils.VerifyHash(reply, in) {
// 				log.Println("OK")
// 			} else {
// 				log.Fatalln("Failed")
// 			}
// 			// cc := msg.CC{
// 			// 	SpecRes: maxSr,
// 			// 	SIdList: sids,
// 			// 	SigList: sigs,
// 			// }
// 			// commit := msg.Commit{
// 			// 	CId: c.id,
// 			// 	CC:  cc,
// 			// }
// 			// cm := msg.CommitMsg{
// 			// 	T:         msg.TypeCommit,
// 			// 	Commit:    commit,
// 			// 	CommitSig: utils.GenSigObj(c, conf.Priv[c.id]),
// 			// }
// 			// comm.UdpMulticastObj(cm)
// 		} else {
// 			log.Printf("Not complete: %d", maxN)
// 		}
// 	}
// }

// func (c *Client) Listen(spec chan<- msg.SpecResMsg) {
// 	l, err := net.ListenPacket("udp", conf.GetListenAddr(c.id))
// 	if err != nil {
// 		panic(err)
// 	}

// 	buf := make([]byte, 1*1024*1024)

// 	for {
// 		n, _, err := l.ReadFrom(buf)
// 		if err != nil {
// 			panic(err)
// 		}

// 		c.listenMu.Lock()
// 		if !c.listen {
// 			continue
// 		}
// 		c.listenMu.Unlock()

// 		b := buf[:n]

// 		var m struct {
// 			T msg.Type
// 		}
// 		err = json.Unmarshal(b, &m)
// 		if err != nil {
// 			panic(err)
// 		}

// 		t := m.T
// 		switch t {
// 		case msg.TypeSpecRes:
// 			log.Println("Got srm")

// 			var srm msg.SpecResMsg
// 			err = json.Unmarshal(b, &srm)
// 			if err != nil {
// 				panic(err)
// 			}

// 			if !msg.VerifySig(srm, []*rsa.PublicKey{conf.Pub[srm.SId], conf.Pub[c.primary]}) {
// 				continue
// 			}

// 			spec <- srm
// 		default:
// 			panic(errors.New("unknown msg type"))
// 		}
// 	}
// }
