package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeterminePrefixForSchema(t *testing.T) {
	tests := []struct {
		name            string
		existingSchema  string
		specifiedPrefix string
		expectedPrefix  string
	}{
		{
			"empty schema",
			"",
			"",
			"",
		},
		{
			"no prefix, none specified",
			`definition user {}`,
			"",
			"",
		},
		{
			"no prefix, one specified",
			`definition user {}`,
			"test",
			"test",
		},
		{
			"prefix found",
			`definition test/user {}`,
			"",
			"test",
		},
		{
			"multiple prefixes found",
			`definition test/user {}
			
			definition something/resource {}`,
			"",
			"",
		},
		{
			"multiple prefixes found, one specified",
			`definition test/user {}
			
			definition something/resource {}`,
			"foobar",
			"foobar",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			found, err := determinePrefixForSchema(test.specifiedPrefix, nil, &test.existingSchema)
			require.NoError(t, err)
			require.Equal(t, test.expectedPrefix, found)
		})
	}
}

func TestRewriteSchema(t *testing.T) {
	tests := []struct {
		name             string
		existingSchema   string
		definitionPrefix string
		expectedSchema   string
	}{
		{
			"empty schema",
			"",
			"",
			"",
		},
		{
			"empty prefix schema",
			"definition user {}",
			"",
			"definition user {}",
		},
		{
			"empty prefix schema with specified",
			`definition user {}
			
			caveat some_caveat(someCondition int) { someCondition == 42 }
			`,
			"test",
			`definition test/user {}

caveat test/some_caveat(someCondition int) {
	someCondition == 42
}`,
		},
		{
			"prefixed schema with specified",
			"definition foo/user {}",
			"test",
			"definition foo/user {}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			found, err := rewriteSchema(test.existingSchema, test.definitionPrefix)
			require.NoError(t, err)
			require.Equal(t, test.expectedSchema, found)
		})
	}
}
