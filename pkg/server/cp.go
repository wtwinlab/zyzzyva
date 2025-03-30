package server

import (
	"bytes"
	"crypto/rsa"
	"log"

	"github.com/myl7/zyzzyva/pkg/conf"
	"github.com/myl7/zyzzyva/pkg/msg"
)

// handleCP processes checkpoint messages
// func (s *Server) handleCP(cpm msg.CPMsg) {
// 	log.Printf("Processing checkpoint from replica %d", cpm.SId)

// 	if !msg.VerifySig(cpm, []*rsa.PublicKey{conf.Pub[cpm.SId]}) {
// 		log.Println("Failed to verify sig")
// 		return
// 	}

// 	if !bytes.Equal(cpm.CP.HistoryHash, s.tentativeCP.cp.HistoryHash) || cpm.CP.Seq != s.tentativeCP.cp.Seq || !bytes.Equal(cpm.CP.StateHash, []byte{}) {
// 		log.Println("Different tentative checkpoint")
// 		return
// 	}

// 	s.tentativeCP.recv[cpm.SId] = true

// 	n := 0
// 	for _, v := range s.tentativeCP.recv {
// 		if v {
// 			n++
// 		}
// 	}

// 	if n >= conf.F+1 {
// 		for i := range s.history {
// 			if bytes.Equal(s.historyHashes[i], cpm.CP.HistoryHash) {
// 				s.history = s.history[i+1:]
// 				s.historyHashes = s.historyHashes[i+1:]
// 				s.committedCP = s.tentativeCP.cp
// 				s.tentativeCP = struct {
// 					cp   msg.CP
// 					recv map[int]bool
// 				}{}

//					log.Println("Set checkpoint")
//					break
//				}
//			}
//		} else {
//			log.Println("Pending checkpoint", n)
//		}
//	}
func (s *Server) handleCP(cpm msg.CPMsg) {
	// Verify signature
	if !msg.VerifySig(cpm, []*rsa.PublicKey{conf.Pub[cpm.SId]}) {
		log.Println("Failed to verify checkpoint signature")
		return
	}

	// Check if we have a tentative checkpoint
	if s.tentativeCP.cp.Seq == 0 {
		log.Println("No tentative checkpoint to compare against")
		return
	}

	// Verify checkpoint matches our tentative checkpoint
	if !bytes.Equal(cpm.CP.HistoryHash, s.tentativeCP.cp.HistoryHash) ||
		cpm.CP.Seq != s.tentativeCP.cp.Seq {
		log.Println("Checkpoint doesn't match our tentative checkpoint")
		return
	}

	// Record this checkpoint message
	s.tentativeCP.recv[cpm.SId] = true

	// Count valid checkpoint messages
	validCount := 0
	for _, valid := range s.tentativeCP.recv {
		if valid {
			validCount++
		}
	}

	// If we have f+1 matching checkpoint messages, commit the checkpoint
	if validCount >= conf.F+1 {
		log.Printf("Committing checkpoint at sequence %d with %d confirmations",
			s.tentativeCP.cp.Seq, validCount)

		// Find the index in history that corresponds to this checkpoint
		checkpointIndex := -1
		for i, hash := range s.historyHashes {
			if bytes.Equal(hash, cpm.CP.HistoryHash) {
				checkpointIndex = i
				break
			}
		}

		if checkpointIndex >= 0 {
			// Truncate history up to this checkpoint
			s.history = s.history[checkpointIndex+1:]
			s.historyHashes = s.historyHashes[checkpointIndex+1:]
			s.committedCP = s.tentativeCP.cp

			// Reset tentative checkpoint
			s.tentativeCP = struct {
				cp   msg.CP
				recv map[int]bool
			}{recv: make(map[int]bool)}

			// Update statistics
			s.statsMu.Lock()
			s.stats.CheckpointsCreated++
			s.statsMu.Unlock()

			log.Println("Checkpoint committed successfully")
		} else {
			log.Println("Error: Could not find checkpoint in history")
		}
	} else {
		log.Printf("Checkpoint has %d/%d confirmations", validCount, conf.F+1)
	}
}
