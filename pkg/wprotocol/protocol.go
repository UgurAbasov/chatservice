package wprotocol

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

const (
	UnitSeparator   = '\x1f'
	RecordSeparator = '\x1e'
)

var ErrInvalidPacket = errors.New("invalid packet format")

type OpCode uint8

const (
	OpMsgSend               OpCode = 1
	OpMsgDeliver            OpCode = 2
	OpMsgEdit               OpCode = 3
	OpMsgEdited             OpCode = 4
	OpMsgDelete             OpCode = 5
	OpMsgDeleted            OpCode = 6
	OpMsgRead               OpCode = 7
	OpMsgStatusUpdate       OpCode = 8
	OpPresenceTypingOn      OpCode = 10
	OpPresenceTypingOff     OpCode = 11
	OpPresenceUpdate        OpCode = 12
	OpNotifyRoomAdded       OpCode = 13
	OpNotifyRoomRemoved     OpCode = 14
	OpFriendRequestReceived OpCode = 15
	OpFriendRequestAccepted OpCode = 16
	OpFriendRemoved         OpCode = 17
	OpWebRTCSignal          OpCode = 20
	OpError                 OpCode = 255
)

type Packet struct {
	Op      OpCode
	Payload []string
}

func Parse(data []byte) (*Packet, error) {
	parts := bytes.SplitN(data, []byte{UnitSeparator}, 2)
	if len(parts) < 1 || len(parts[0]) == 0 {
		return nil, ErrInvalidPacket
	}
	op, err := strconv.ParseUint(string(parts[0]), 10, 8)
	if err != nil {
		return nil, ErrInvalidPacket
	}
	var payload []string
	if len(parts) == 2 {
		payload = strings.Split(string(parts[1]), string(RecordSeparator))
	}
	return &Packet{Op: OpCode(op), Payload: payload}, nil
}

func Build(op OpCode, params ...string) []byte {
	opStr := strconv.Itoa(int(op))
	payload := strings.Join(params, string(RecordSeparator))
	buf := make([]byte, 0, len(opStr)+1+len(payload))
	buf = append(buf, opStr...)
	buf = append(buf, UnitSeparator)
	buf = append(buf, payload...)
	return buf
}