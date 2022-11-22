package main

import (
	"fmt"

	"github.com/gogo/protobuf/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// marshalAny marshals Google's proto.Message to Gogo's Any.
func marshalAny(in proto.Message) (*types.Any, error) {
	data, err := proto.Marshal(in)
	if err != nil {
		return nil, err
	}
	url := string(in.ProtoReflect().Descriptor().FullName())
	if err != nil {
		return nil, err
	}
	return &types.Any{Value: data, TypeUrl: url}, nil
}

// unmarshalAny unmarshals Gogo's Any to Google's proto.Message
func unmarshalAny(in *types.Any) (proto.Message, error) {
	mt, err := protoregistry.GlobalTypes.FindMessageByURL(in.TypeUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal %+v: %w", in, err)
	}

	out := mt.New().Interface()
	err = proto.Unmarshal(in.Value, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
