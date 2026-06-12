package sael

import (
	"github.com/vmihailenco/msgpack/v5"
)

// MarshalFrameJSON returns the JSON encoding of a frame.
func MarshalFrameJSON(frame map[string]any) ([]byte, error) {
	return marshalJSON(frame)
}

// MarshalFrameMsgpack returns the MessagePack encoding of a frame.
//
// MessagePack is opt-in via the WebSocket subprotocol
// "application/x-sael-msgpack" — see spec §4.2.
func MarshalFrameMsgpack(frame map[string]any) ([]byte, error) {
	return msgpack.Marshal(frame)
}

// UnmarshalFrameMsgpack decodes a MessagePack frame.
func UnmarshalFrameMsgpack(data []byte) (map[string]any, error) {
	var out map[string]any
	if err := msgpack.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// MsgpackSubprotocol is the WebSocket subprotocol name advertised
// for MessagePack binary frames.
const MsgpackSubprotocol = "application/x-sael-msgpack"

// JSONSubprotocol is the default subprotocol name.
const JSONSubprotocol = "application/x-sael-json"
