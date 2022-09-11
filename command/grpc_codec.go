package command

import (
	"fmt"
	"github.com/golang/protobuf/proto"
)

// name is the name registered for the proto compressor.
const name = "proto"

type vtProtoCodec struct{}

type vtProtoMessage interface {
	MarshalVT() ([]byte, error)
	UnmarshalVT([]byte) error
}

func (vtProtoCodec) Marshal(v any) ([]byte, error) {
	vt, ok := v.(vtProtoMessage)
	if ok {
		return vt.MarshalVT()
	}

	vv, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("failed to marshal, message is %T, want proto.Message", v)
	}
	return proto.Marshal(vv)
}

func (vtProtoCodec) Unmarshal(data []byte, v any) error {
	vt, ok := v.(vtProtoMessage)
	if ok {
		return vt.UnmarshalVT(data)
	}

	vv, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("failed to unmarshal, message is %T, want proto.Message", v)
	}
	return proto.Unmarshal(data, vv)
}

func (vtProtoCodec) Name() string {
	return name
}
