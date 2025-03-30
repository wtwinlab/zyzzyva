package msg

// LocalCommit message is sent by replicas in response to a Commit message
type LocalCommit struct {
	View        int    // Current view
	ReqHash     []byte // Hash of the request
	HistoryHash []byte // History hash
	SId         int    // Server ID
	CId         int    // Client ID that sent the commit
}

// LocalCommitMsg wraps the LocalCommit with message type and signature
type LocalCommitMsg struct {
	T              Type
	LocalCommit    LocalCommit
	LocalCommitSig []byte
}

func (lcm LocalCommitMsg) getAllObj() []interface{} {
	return []interface{}{lcm.LocalCommit}
}

func (lcm LocalCommitMsg) getAllSig() [][]byte {
	return [][]byte{lcm.LocalCommitSig}
}
