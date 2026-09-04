package commands

import (
	"encoding/json"
	"errors"
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

const (
	resourceFormat     = "`<type>:<id>`"
	subjectFormat      = "`<type>:<id>` or `<type>:<id>#<relation>`"
	resourceTypeFormat = "`<type>`"
	subjectTypeFormat  = "`<type>` or `<type>#<relation>`"
)

// invalidArgError returns a ValidationError describing an argument that is not
// in the expected format, so that the command's usage is printed alongside it.
func invalidArgError(kind, value, format string) error {
	return ValidationError{error: fmt.Errorf("invalid %s %q: expected format %s", kind, value, format)}
}

// ParseResource parses the given resource string into its object type and
// object ID, if valid.
func ParseResource(s string) (objectType, objectID string, err error) {
	if err := stringz.SplitExact(s, ":", &objectType, &objectID); err != nil {
		return "", "", invalidArgError("resource", s, resourceFormat)
	}
	if objectType == "" || objectID == "" {
		return "", "", invalidArgError("resource", s, resourceFormat)
	}
	return objectType, objectID, nil
}

// ParseSubject parses the given subject string into its namespace, object ID
// and relation, if valid.
func ParseSubject(s string) (namespace, id, relation string, err error) {
	if err := stringz.SplitExact(s, ":", &namespace, &id); err != nil {
		return "", "", "", invalidArgError("subject", s, subjectFormat)
	}
	if strings.Contains(id, "#") {
		if err := stringz.SplitExact(id, "#", &id, &relation); err != nil || relation == "" {
			return "", "", "", invalidArgError("subject", s, subjectFormat)
		}
	}
	if namespace == "" || id == "" {
		return "", "", "", invalidArgError("subject", s, subjectFormat)
	}
	return namespace, id, relation, nil
}

// ParseResourceType parses a bare type reference of the form `namespace`.
func ParseResourceType(s string) (namespace string, err error) {
	if s == "" || strings.ContainsAny(s, ":#") {
		return "", invalidArgError("resource type", s, resourceTypeFormat)
	}
	return s, nil
}

// ParseType parses a type reference of the form `namespace#relation`, where the
// relation is optional.
func ParseType(s string) (namespace, relation string, err error) {
	if strings.Contains(s, ":") {
		return "", "", invalidArgError("subject type", s, subjectTypeFormat)
	}
	namespace, relation, _ = strings.Cut(s, "#")
	if namespace == "" {
		return "", "", invalidArgError("subject type", s, subjectTypeFormat)
	}
	return namespace, relation, nil
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
	var obj any
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

// ValidationError is used to wrap errors that are cobra validation errors. It should be used to
// wrap the Command.PositionalArgs function in order to be able to determine if the error is a validation error.
// This is used to determine if an error should print the usage string. Unfortunately Cobra parameter parsing
// and parameter validation are handled differently, and the latter does not trigger calling Command.FlagErrorFunc
type ValidationError struct {
	error
}

func (ve ValidationError) Is(err error) bool {
	var validationError ValidationError
	return errors.As(err, &validationError)
}

// ValidationWrapper is used to be able to determine if an error is a validation error.
func ValidationWrapper(f cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := f(cmd, args); err != nil {
			return ValidationError{error: err}
		}

		return nil
	}
}
