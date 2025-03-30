package server

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha512"
	"log"

	"github.com/myl7/zyzzyva/pkg/comm"
	"github.com/myl7/zyzzyva/pkg/conf"
	"github.com/myl7/zyzzyva/pkg/msg"
	"github.com/myl7/zyzzyva/pkg/utils"
)

// handleOrderReq processes ordered requests from the primary
func (s *Server) handleOrderReq(orm msg.OrderReqMsg) {
	// Check if this message is coming from the current primary
	primaryID := s.view % conf.N
	if primaryID != orm.OrderReq.View%conf.N {
		log.Printf("Received order request from non-primary replica %d, ignoring", orm.OrderReq.View%conf.N)
		return
	}

	// Verify the signatures
	if !msg.VerifySig(orm, []*rsa.PublicKey{conf.Pub[primaryID], conf.Pub[orm.Req.CId]}) {
		log.Println("Failed to verify order request signatures")
		return
	}

	// Verify request hash
	reqDigest := utils.GenHashObj(orm.Req)
	if !bytes.Equal(reqDigest, orm.OrderReq.ReqHash) {
		log.Println("Request hash mismatch")
		return
	}

	// Verify sequence number
	if orm.OrderReq.Seq != s.nextSeq {
		log.Printf("Sequence number mismatch: got %d, expected %d",
			orm.OrderReq.Seq, s.nextSeq)
		return
	}

	// Verify history hash
	histHash := []byte{}
	if len(s.historyHashes) > 0 {
		histHash = s.historyHashes[len(s.historyHashes)-1]
	} else if len(s.committedCP.HistoryHash) > 0 {
		histHash = s.committedCP.HistoryHash
	}

	hasher := sha512.New()
	hasher.Write(histHash)
	hasher.Write(reqDigest)
	newHistHash := hasher.Sum(nil)

	if !bytes.Equal(newHistHash, orm.OrderReq.HistoryHash) {
		log.Println("History hash mismatch")
		return
	}

	// Process the request
	s.history = append(s.history, orm.Req)
	s.historyHashes = append(s.historyHashes, newHistHash)
	s.nextSeq++

	// Check if checkpoint is needed
	if len(s.history) >= conf.CPInterval && len(s.history)%conf.CPInterval == 0 {
		s.createCheckpoint()
	}

	// Generate and send response to client
	rep := utils.GenHash(orm.Req.Data)
	repDigest := utils.GenHash(rep)

	// Create speculative response
	sr := msg.SpecRes{
		View:        s.view,
		Seq:         orm.OrderReq.Seq,
		HistoryHash: newHistHash,
		ResHash:     repDigest,
		CId:         orm.Req.CId,
		Timestamp:   orm.Req.Timestamp,
	}
	srs := utils.GenSigObj(sr, conf.Priv[s.id])

	// Create response message
	srm := msg.SpecResMsg{
		T:           msg.TypeSpecRes,
		SpecRes:     sr,
		SpecResSig:  srs,
		SId:         s.id,
		Reply:       rep,
		OrderReq:    orm.OrderReq,
		OrderReqSig: orm.OrderReqSig,
	}

	// Send to client
	comm.UdpSendObj(srm, orm.Req.CId)
}

// func (s *Server) handleOrderReq(orm msg.OrderReqMsg) {

// 	log.Printf("Processing ordered request with sequence number %d", orm.OrderReq.Seq)
// 	if s.view%conf.N == s.id {
// 		log.Println("Be primary")
// 		return
// 	}

// 	if !msg.VerifySig(orm, []*rsa.PublicKey{conf.Pub[s.view%conf.N], conf.Pub[orm.Req.CId]}) {
// 		log.Println("Failed to verify sig")
// 		return
// 	}

// 	r := orm.Req
// 	or := orm.OrderReq
// 	ors := orm.OrderReqSig
// 	rd := utils.GenHashObj(r)

// 	if !bytes.Equal(rd, or.ReqHash) {
// 		log.Println("Failed to check req hash")
// 		return
// 	}

// 	if or.Seq != s.nextSeq {
// 		log.Println("Failed to check seq")
// 		return
// 	}

// 	hh := sha512.New()
// 	if len(s.historyHashes) > 0 {
// 		hh.Write(s.historyHashes[len(s.historyHashes)-1])
// 	} else if len(s.committedCP.HistoryHash) > 0 {
// 		hh.Write(s.committedCP.HistoryHash)
// 	}
// 	hh.Write(rd)
// 	if !bytes.Equal(hh.Sum(nil), or.HistoryHash) {
// 		log.Println("Failed to check history hash")
// 		return
// 	}

// 	toCP := false
// 	if len(s.history) >= 2*conf.CPInterval {
// 		log.Println("History is full")
// 		// If checkpoint is buggy, we will still continue
// 		// return
// 	} else if len(s.history) == conf.CPInterval {
// 		toCP = true
// 	}

// 	s.history = append(s.history, r)
// 	s.historyHashes = append(s.historyHashes, hh.Sum(nil))
// 	seq := s.nextSeq
// 	s.nextSeq += 1

// 	if toCP {
// 		cp := msg.CP{
// 			Seq:         s.nextSeq,
// 			HistoryHash: hh.Sum(nil),
// 			StateHash:   []byte{},
// 		}
// 		s.tentativeCP = struct {
// 			cp   msg.CP
// 			recv map[int]bool
// 		}{cp: cp, recv: make(map[int]bool)}

// 		go func() {
// 			cps := utils.GenSigObj(cp, conf.Priv[s.id])
// 			cpm := msg.CPMsg{
// 				T:     msg.TypeCP,
// 				SId:   s.id,
// 				CP:    cp,
// 				CPSig: cps,
// 			}

// 			comm.UdpMulticastObj(cpm)
// 		}()
// 	}

// 	rep := utils.GenHash(orm.Req.Data)
// 	repd := utils.GenHash(rep)
// 	sr := msg.SpecRes{
// 		View:        s.view,
// 		Seq:         seq,
// 		HistoryHash: s.historyHashes[len(s.historyHashes)-1],
// 		ResHash:     repd,
// 		CId:         r.CId,
// 		Timestamp:   r.Timestamp,
// 	}
// 	srs := utils.GenSigObj(sr, conf.Priv[s.id])
// 	srm := msg.SpecResMsg{
// 		T:           msg.TypeSpecRes,
// 		SpecRes:     sr,
// 		SpecResSig:  srs,
// 		SId:         s.id,
// 		Reply:       rep,
// 		OrderReq:    or,
// 		OrderReqSig: ors,
// 	}

// 	comm.UdpSendObj(srm, r.CId)
// }
