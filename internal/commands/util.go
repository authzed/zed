package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TylerBrock/colorjson"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/authzed/authzed-go/pkg/requestmeta"
)

// ParseSubject parses the given subject string into its namespace, object ID
// and relation, if valid.
func ParseSubject(s string) (namespace, id, relation string, err error) {
	err = stringz.SplitExact(s, ":", &namespace, &id)
	if err != nil {
		return
	}
	err = stringz.SplitExact(id, "#", &id, &relation)
	if err != nil {
		relation = ""
		err = nil
	}
	return
}

// ParseType parses a type reference of the form `namespace#relaion`.
func ParseType(s string) (namespace, relation string) {
	namespace, relation, _ = strings.Cut(s, "#")
	return
}

// GetCaveatContext returns the entered caveat caveat, if any.
func GetCaveatContext(cmd *cobra.Command) (*structpb.Struct, error) {
	contextString := cobrautil.MustGetString(cmd, "caveat-context")
	if len(contextString) == 0 {
		return nil, nil
	}

	return ParseCaveatContext(contextString)
}

// ParseCaveatContext parses the given context JSON string into caveat context,
// if valid.
func ParseCaveatContext(contextString string) (*structpb.Struct, error) {
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

// PrettyProto returns the given protocol buffer formatted into pretty text.
func PrettyProto(m proto.Message) ([]byte, error) {
	encoded, err := protojson.Marshal(m)
	if err != nil {
		return nil, err
	}
	var obj interface{}
	err = json.Unmarshal(encoded, &obj)
	if err != nil {
		panic("protojson decode failed: " + err.Error())
	}

	f := colorjson.NewFormatter()
	f.Indent = 2
	pretty, err := f.Marshal(obj)
	if err != nil {
		panic("colorjson encode failed: " + err.Error())
	}

	return pretty, nil
}

// InjectRequestID adds the value of the --request-id flag to the
// context of the given command.
func InjectRequestID(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	requestID := cobrautil.MustGetString(cmd, "request-id")
	if ctx != nil && requestID != "" {
		cmd.SetContext(requestmeta.WithRequestID(ctx, requestID))
	}

	return nil
}
