package server

import (
	"log"

	"github.com/myl7/zyzzyva/pkg/comm"
	"github.com/myl7/zyzzyva/pkg/conf"
	"github.com/myl7/zyzzyva/pkg/msg"
	"github.com/myl7/zyzzyva/pkg/utils"
)

// handleCommit processes commit messages from clients

func (s *Server) handleCommit(cm msg.CommitMsg) {
	// Verify signatures
	if !msg.VerifySig(cm, cm.GetAllPub()) {
		log.Println("Failed to verify commit signatures")
		return
	}

	// Process the commit certificate
	// For a real implementation, we would:
	// 1. Verify the commit certificate has 2f+1 valid signatures
	// 2. Check if we already executed this request
	// 3. If not, execute it and update our state
	// 4. Send a local-commit message back to the client

	log.Printf("Processing commit from client %d", cm.Commit.CId)

	// For this simplified implementation, just send a local-commit response
	localCommit := msg.LocalCommit{
		View:        s.view,
		ReqHash:     []byte{}, // Should be filled with actual request hash
		HistoryHash: []byte{}, // Should be filled with actual history hash
		SId:         s.id,
		CId:         cm.Commit.CId,
	}

	localCommitSig := utils.GenSigObj(localCommit, conf.Priv[s.id])
	localCommitMsg := msg.LocalCommitMsg{
		T:              msg.TypeLocalCommit,
		LocalCommit:    localCommit,
		LocalCommitSig: localCommitSig,
	}

	comm.UdpSendObj(localCommitMsg, cm.Commit.CId)
}
