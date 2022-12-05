package main

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"
)

func parseCaveatContext(contextString string) (*structpb.Struct, error) {
	contextMap := map[string]any{}
	err := json.Unmarshal([]byte(contextString), &contextMap)
	if err != nil {
		return nil, fmt.Errorf("invalid caveat context JSON: %w", err)
	}

	context, err := structpb.NewStruct(contextMap)
	if err != nil {
		return nil, fmt.Errorf("could not construct caveat context: %w", err)
	}
	return context, err
}
