package commands

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

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
