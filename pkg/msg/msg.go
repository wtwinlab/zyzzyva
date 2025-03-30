package msg

import "encoding/json"

type Type int

const (
	TypeReq Type = iota
	TypeOrderReq
	TypeSpecRes
	TypeCP
	TypeCommit

	TypeLocalCommit // Adding missing message type
	TypeViewChange
	TypeIHateThePrimary
	TypeNewView
	TypeViewConfirm
	TypeFillHole
	TypeConfirmReq
	TypePOM
)

type Msg struct {
	T Type
}

func DeType(b []byte) (Type, error) {
	var m Msg
	err := json.Unmarshal(b, &m)
	if err != nil {
		return 0, err
	}

	return m.T, nil
}
