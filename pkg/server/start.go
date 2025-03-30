package server

import (
	"crypto/rsa"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/myl7/zyzzyva/pkg/comm"
	"github.com/myl7/zyzzyva/pkg/conf"
	"github.com/myl7/zyzzyva/pkg/msg"
	"github.com/myl7/zyzzyva/pkg/utils"
)

// Server enhances the Zyzzyva server with configurable parameters
type Server struct {
	// Basic server state
	id            int
	stateMu       sync.Mutex
	history       []msg.Req
	historyHashes [][]byte
	maxCC         int
	view          int
	nextSeq       int

	// Checkpoint state
	committedCP msg.CP
	tentativeCP struct {
		cp   msg.CP
		recv map[int]bool
	}

	// Client response caching
	respCache map[int]struct {
		state     int
		timestamp int64
	}

	// Batching support
	batchSize     int
	batchTimeout  time.Duration
	currentBatch  []msg.Req
	currentReqSig [][]byte
	batchTimer    *time.Timer
	batchMu       sync.Mutex

	// Performance metrics
	stats     ServerStats
	statsMu   sync.Mutex
	startTime time.Time
}

// ServerStats tracks server performance metrics
type ServerStats struct {
	TotalRequests      int     `json:"total_requests"`
	BatchesProcessed   int     `json:"batches_processed"`
	AvgBatchSize       float64 `json:"avg_batch_size"`
	CheckpointsCreated int     `json:"checkpoints_created"`
	ViewChanges        int     `json:"view_changes"`
}

// ServerOptions allows customization of server behavior
type ServerOptions struct {
	BatchSize    int
	BatchTimeout time.Duration
	StartView    int
}

// NewServer creates a new enhanced Zyzzyva server
func NewServer(id int, options *ServerOptions) *Server {
	// Use default options if none provided
	if options == nil {
		options = &ServerOptions{
			BatchSize:    conf.BatchSize,
			BatchTimeout: conf.BatchTimeout,
			StartView:    0,
		}
	}

	s := &Server{
		id: id,
		respCache: make(map[int]struct {
			state     int
			timestamp int64
		}),
		batchSize:    options.BatchSize,
		batchTimeout: options.BatchTimeout,
		view:         options.StartView,
		startTime:    time.Now(),
	}

	// Initialize batching timer
	s.batchTimer = time.AfterFunc(s.batchTimeout, s.processBatch)
	s.batchTimer.Stop() // Don't start until first request

	return s
}

// Run starts the server operation
func (s *Server) Run() {
	log.Printf("Server %d starting with batch size %d and timeout %v",
		s.id, s.batchSize, s.batchTimeout)

	// Print if this server is the primary
	if s.view%conf.N == s.id {
		log.Printf("Server %d is the primary for view %d", s.id, s.view)
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.listen()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.listenMulticast()
	}()

	wg.Wait()
}

// PrintStats outputs the server statistics
func (s *Server) PrintStats() {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	// Calculate derived metrics
	if s.stats.BatchesProcessed > 0 {
		s.stats.AvgBatchSize = float64(s.stats.TotalRequests) / float64(s.stats.BatchesProcessed)
	}

	// Get runtime
	runtime := time.Since(s.startTime)

	fmt.Printf("\n======= Server %d Statistics =======\n", s.id)
	fmt.Printf("Runtime:              %v\n", runtime)
	fmt.Printf("Total Requests:       %d\n", s.stats.TotalRequests)
	fmt.Printf("Batches Processed:    %d\n", s.stats.BatchesProcessed)
	fmt.Printf("Average Batch Size:   %.2f\n", s.stats.AvgBatchSize)
	fmt.Printf("Checkpoints Created:  %d\n", s.stats.CheckpointsCreated)
	fmt.Printf("View Changes:         %d\n", s.stats.ViewChanges)
	fmt.Printf("Throughput:           %.2f requests/second\n",
		float64(s.stats.TotalRequests)/runtime.Seconds())
	fmt.Printf("Primary:              %v\n", s.view%conf.N == s.id)
	fmt.Printf("Current View:         %d\n", s.view)
	fmt.Printf("===================================\n\n")
}

// listen handles incoming network messages on the server's address
func (s *Server) listen() {
	comm.UdpListen(conf.GetListenAddr(s.id), s.handle)
}

// listenMulticast handles incoming multicast messages
func (s *Server) listenMulticast() {
	comm.UdpListenMulticast(s.handle)
}

// handle processes incoming messages
func (s *Server) handle(b []byte) {
	t, err := msg.DeType(b)
	if err != nil {
		log.Println("Bad msg without type")
		return
	}

	switch t {
	case msg.TypeReq:
		if conf.VerboseLogging {
			log.Println("Got request message")
		}

		var rm msg.ReqMsg
		err := json.Unmarshal(b, &rm)
		if err != nil {
			log.Printf("Error unmarshalling request: %v", err)
			return
		}

		// Only handle requests if we're the primary
		if s.view%conf.N == s.id {
			s.handleReqWithBatching(rm)
		} else {
			s.forwardToPrimary(rm)
		}

	case msg.TypeOrderReq:
		if conf.VerboseLogging {
			log.Println("Got ordered request")
		}

		var orm msg.OrderReqMsg
		err := json.Unmarshal(b, &orm)
		if err != nil {
			log.Printf("Error unmarshalling ordered request: %v", err)
			return
		}

		s.stateMu.Lock()
		s.handleOrderReq(orm)
		s.stateMu.Unlock()

	case msg.TypeCP:
		if conf.VerboseLogging {
			log.Println("Got checkpoint message")
		}

		var cpm msg.CPMsg
		err := json.Unmarshal(b, &cpm)
		if err != nil {
			log.Printf("Error unmarshalling checkpoint: %v", err)
			return
		}

		s.stateMu.Lock()
		s.handleCP(cpm)
		s.stateMu.Unlock()

	case msg.TypeCommit:
		if conf.VerboseLogging {
			log.Println("Got commit message")
		}

		var cm msg.CommitMsg
		err := json.Unmarshal(b, &cm)
		if err != nil {
			log.Printf("Error unmarshalling commit: %v", err)
			return
		}

		s.stateMu.Lock()
		s.handleCommit(cm)
		s.statsMu.Unlock()

	default:
		log.Printf("Unknown message type: %d", t)
	}
}

// handleReqWithBatching handles client requests with batching support
func (s *Server) handleReqWithBatching(rm msg.ReqMsg) {
	// Verify the request signature
	if !msg.VerifySig(rm, []*rsa.PublicKey{conf.Pub[rm.Req.CId]}) {
		log.Println("Failed to verify request signature")
		return
	}

	// Check client timestamp to prevent replay attacks
	s.stateMu.Lock()
	if c, ok := s.respCache[rm.Req.CId]; ok && c.timestamp >= rm.Req.Timestamp {
		log.Println("Request has outdated timestamp")
		s.stateMu.Unlock()
		return
	} else {
		s.respCache[rm.Req.CId] = struct {
			state     int
			timestamp int64
		}{timestamp: rm.Req.Timestamp}
	}
	s.stateMu.Unlock()

	// Add request to batch
	s.batchMu.Lock()
	defer s.batchMu.Unlock()

	// Start timer on first request
	if len(s.currentBatch) == 0 && s.batchSize > 1 {
		s.batchTimer.Reset(s.batchTimeout)
	}

	// Add to current batch
	s.currentBatch = append(s.currentBatch, rm.Req)
	s.currentReqSig = append(s.currentReqSig, rm.ReqSig)

	// Process batch if it's full
	if len(s.currentBatch) >= s.batchSize {
		s.batchTimer.Stop()
		go s.processBatch()
	}
}

// processBatch orders and processes a batch of requests
func (s *Server) processBatch() {
	s.batchMu.Lock()

	// If batch is empty, just reset and return
	if len(s.currentBatch) == 0 {
		s.batchMu.Unlock()
		return
	}

	// Get the batch and clear for next one
	batch := s.currentBatch
	reqSigs := s.currentReqSig
	s.currentBatch = nil
	s.currentReqSig = nil

	s.batchMu.Unlock()

	// Process the batch
	log.Printf("Processing batch of %d requests", len(batch))

	s.statsMu.Lock()
	s.stats.BatchesProcessed++
	s.stats.TotalRequests += len(batch)
	s.statsMu.Unlock()

	// Take state lock to process the batch
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	// Get sequence number for this batch
	seq := s.nextSeq
	s.nextSeq += len(batch)

	// Process each request in the batch
	for i, req := range batch {
		// Add request to history
		s.history = append(s.history, req)

		// Update history hash
		reqDigest := utils.GenHashObj(req)
		histHash := []byte{}
		if len(s.historyHashes) > 0 {
			histHash = s.historyHashes[len(s.historyHashes)-1]
		}

		hasher := sha512.New()
		hasher.Write(histHash)
		hasher.Write(reqDigest)
		newHistHash := hasher.Sum(nil)
		s.historyHashes = append(s.historyHashes, newHistHash)

		// Create ordered request message
		or := msg.OrderReq{
			View:        s.view,
			Seq:         seq + i,
			HistoryHash: newHistHash,
			ReqHash:     reqDigest,
			Extra:       conf.ExtraNonDeterministic,
		}
		ors := utils.GenSigObj(or, conf.Priv[s.id])

		// Create ordered request message
		orm := msg.OrderReqMsg{
			T:           msg.TypeOrderReq,
			OrderReq:    or,
			OrderReqSig: ors,
			Req:         req,
			ReqSig:      reqSigs[i],
		}

		// Send to all replicas
		go comm.UdpMulticastObj(orm)

		// Generate and send response to client
		go func(r msg.Req, o msg.OrderReq, oSig []byte) {
			// Generate response
			rep := utils.GenHash(r.Data)
			repDigest := utils.GenHash(rep)

			// Create speculative response
			sr := msg.SpecRes{
				View:        s.view,
				Seq:         o.Seq,
				HistoryHash: o.HistoryHash,
				ResHash:     repDigest,
				CId:         r.CId,
				Timestamp:   r.Timestamp,
			}
			srs := utils.GenSigObj(sr, conf.Priv[s.id])

			// Create response message
			srm := msg.SpecResMsg{
				T:           msg.TypeSpecRes,
				SpecRes:     sr,
				SpecResSig:  srs,
				SId:         s.id,
				Reply:       rep,
				OrderReq:    o,
				OrderReqSig: oSig,
			}

			// Send to client
			comm.UdpSendObj(srm, r.CId)
		}(req, or, ors)
	}

	// Check if we need to create a checkpoint
	if len(s.history) >= conf.CPInterval && len(s.history)%conf.CPInterval == 0 {
		s.createCheckpoint()
	}
}

// createCheckpoint creates a new checkpoint
func (s *Server) createCheckpoint() {
	log.Println("Creating checkpoint")

	// Update statistics
	s.statsMu.Lock()
	s.stats.CheckpointsCreated++
	s.statsMu.Unlock()

	// Create checkpoint
	cp := msg.CP{
		Seq:         s.nextSeq,
		HistoryHash: s.historyHashes[len(s.historyHashes)-1],
		StateHash:   []byte{}, // Empty state hash for this simple implementation
	}

	s.tentativeCP = struct {
		cp   msg.CP
		recv map[int]bool
	}{cp: cp, recv: make(map[int]bool)}

	// Sign and send checkpoint message
	cps := utils.GenSigObj(cp, conf.Priv[s.id])
	cpm := msg.CPMsg{
		T:     msg.TypeCP,
		SId:   s.id,
		CP:    cp,
		CPSig: cps,
	}

	comm.UdpMulticastObj(cpm)
}

// forwardToPrimary forwards client requests to the primary
func (s *Server) forwardToPrimary(rm msg.ReqMsg) {
	primaryId := s.view % conf.N
	log.Printf("Forwarding request from client %d to primary %d", rm.Req.CId, primaryId)
	comm.UdpSendObj(rm, primaryId)
}

// func (s *Server) Run() {
// 	var wg sync.WaitGroup

// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()

// 		s.listen()
// 	}()

// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()

// 		s.listenMulticast()
// 	}()

// 	wg.Wait()
// }

// func (s *Server) listen() {
// 	comm.UdpListen(conf.GetListenAddr(s.id), s.handle)
// }

// func (s *Server) listenMulticast() {
// 	comm.UdpListenMulticast(s.handle)
// }

// func (s *Server) handle(b []byte) {
// 	s.stateMu.Lock()
// 	t, err := msg.DeType(b)
// 	if err != nil {
// 		log.Fatalln("Bad msg without type")
// 	}

// 	switch t {
// 	case msg.TypeReq:
// 		log.Println("Got rm")

// 		var rm msg.ReqMsg
// 		err := json.Unmarshal(b, &rm)
// 		if err != nil {
// 			panic(err)
// 		}

// 		s.handleReq(rm)
// 	case msg.TypeOrderReq:
// 		log.Println("Got orm")

// 		var orm msg.OrderReqMsg
// 		err := json.Unmarshal(b, &orm)
// 		if err != nil {
// 			panic(err)
// 		}

// 		s.handleOrderReq(orm)
// 	case msg.TypeCP:
// 		log.Println("Got cpm")

// 		var cpm msg.CPMsg
// 		err := json.Unmarshal(b, &cpm)
// 		if err != nil {
// 			panic(err)
// 		}

// 		s.handleCP(cpm)
// 	case msg.TypeCommit:
// 		log.Println("Got cm")

// 		var cm msg.CommitMsg
// 		err := json.Unmarshal(b, &cm)
// 		if err != nil {
// 			panic(err)
// 		}

// 		s.handleCommit(cm)
// 	default:
// 		panic(errors.New("unknown msg type"))
// 	}
// 	s.stateMu.Unlock()
// }
