//go:build wasm
// +build wasm

// To Run:
// 1) Install wasmbrowsertest: `go install github.com/agnivade/wasmbrowsertest@latest`
// 2) Run: `GOOS=js GOARCH=wasm go test -exec wasmbrowsertest`

package main

import (
	"testing"

	devinterface "github.com/authzed/spicedb/pkg/proto/developer/v1"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestZedCommand(t *testing.T) {
	requestCtx := &devinterface.RequestContext{
		Schema: `definition user {}
		
		definition document {
			relation viewer: user
			permission view = viewer
		}`,
	}

	m := jsonpb.Marshaler{}
	encodedContext, err := m.MarshalToString(requestCtx)
	require.NoError(t, err)

	result := runZedCommand(encodedContext, []string{"permission", "check", "document:firstdoc", "view", "user:tom"})
	require.Contains(t, result.Output, "false")

	updatedContext := &devinterface.RequestContext{}
	err = protojson.Unmarshal([]byte(result.UpdatedContext), updatedContext)
	require.NoError(t, err)

	require.Contains(t, updatedContext.Schema, "definition document")
}
