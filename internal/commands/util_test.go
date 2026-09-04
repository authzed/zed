package commands

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestParseResource(t *testing.T) {
	tests := []struct {
		input          string
		wantObjectType string
		wantObjectID   string
		wantErr        bool
	}{
		{input: "test/resource:foo", wantObjectType: "test/resource", wantObjectID: "foo"},
		{input: "resource:foo", wantObjectType: "resource", wantObjectID: "foo"},
		{input: "test/resource", wantErr: true},
		{input: "test/resource:foo:bar", wantErr: true},
		{input: "test/resource:", wantErr: true},
		{input: ":foo", wantErr: true},
		{input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			objectType, objectID, err := ParseResource(tt.input)
			if tt.wantErr {
				requireValidationError(t, err, "invalid resource")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantObjectType, objectType)
			require.Equal(t, tt.wantObjectID, objectID)
		})
	}
}

func TestParseSubject(t *testing.T) {
	tests := []struct {
		input         string
		wantNamespace string
		wantID        string
		wantRelation  string
		wantErr       bool
	}{
		{input: "test/user:jimmy", wantNamespace: "test/user", wantID: "jimmy"},
		{input: "test/group:eng#member", wantNamespace: "test/group", wantID: "eng", wantRelation: "member"},
		{input: "test/user", wantErr: true},
		{input: "test/user:jimmy:extra", wantErr: true},
		{input: "test/user:", wantErr: true},
		{input: ":jimmy", wantErr: true},
		{input: "test/group:eng#", wantErr: true},
		{input: "test/group:eng#member#extra", wantErr: true},
		{input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			namespace, id, relation, err := ParseSubject(tt.input)
			if tt.wantErr {
				requireValidationError(t, err, "invalid subject")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantNamespace, namespace)
			require.Equal(t, tt.wantID, id)
			require.Equal(t, tt.wantRelation, relation)
		})
	}
}

func TestParseResourceType(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{input: "test/resource"},
		{input: "resource"},
		{input: "test/resource:foo", wantErr: true},
		{input: "test/resource#viewer", wantErr: true},
		{input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			namespace, err := ParseResourceType(tt.input)
			if tt.wantErr {
				requireValidationError(t, err, "invalid resource type")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.input, namespace)
		})
	}
}

func TestParseType(t *testing.T) {
	tests := []struct {
		input         string
		wantNamespace string
		wantRelation  string
		wantErr       bool
	}{
		{input: "test/user", wantNamespace: "test/user"},
		{input: "test/group#member", wantNamespace: "test/group", wantRelation: "member"},
		{input: "test/user:jimmy", wantErr: true},
		{input: "#member", wantErr: true},
		{input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			namespace, relation, err := ParseType(tt.input)
			if tt.wantErr {
				requireValidationError(t, err, "invalid subject type")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantNamespace, namespace)
			require.Equal(t, tt.wantRelation, relation)
		})
	}
}

// requireValidationError asserts that the given error is a ValidationError, so
// that the command's usage is printed, and that it names the offending argument.
func requireValidationError(t *testing.T, err error, contains string) {
	t.Helper()

	var validationError ValidationError
	require.ErrorAs(t, err, &validationError)
	require.ErrorContains(t, err, contains)
}

func TestValidationWrapper(t *testing.T) {
	tests := []struct {
		name           string
		positionalArgs cobra.PositionalArgs
		args           []string
		wantErr        bool
	}{
		{
			name:           "valid args",
			positionalArgs: cobra.MaximumNArgs(2),
			args:           []string{"arg1", "arg2"},
			wantErr:        false,
		},
		{
			name:           "invalid args",
			positionalArgs: cobra.MaximumNArgs(2),
			args:           []string{"arg1", "arg2", "arg3"},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidationWrapper(tt.positionalArgs)(nil, tt.args)
			if tt.wantErr {
				var validationError ValidationError
				require.ErrorAs(t, err, &validationError)
				require.Error(t, validationError.error)
				require.ErrorContains(t, validationError.error, "accepts at most")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
