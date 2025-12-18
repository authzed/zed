package backupformat

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func TestNewEncoder(t *testing.T) {
	tests := []struct {
		name        string
		schema      string
		token       *v1.ZedToken
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid encoder creation",
			schema:      "definition user {}",
			token:       &v1.ZedToken{Token: "dGVzdF90b2tlbl8xMjM="},
			expectError: false,
		},
		{
			name:        "empty schema",
			schema:      "",
			token:       &v1.ZedToken{Token: "dGVzdF90b2tlbl8xMjM="},
			expectError: false,
		},
		{
			name:        "complex schema",
			schema:      "definition user {}\ndefinition document { relation viewer: user }",
			token:       &v1.ZedToken{Token: "Y29tcGxleF90b2tlbl8xMjM="},
			expectError: false,
		},
		{
			name:        "empty token",
			schema:      "definition user {}",
			token:       &v1.ZedToken{Token: ""},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewOcfEncoder(&buf)
			err := enc.WriteSchema(tt.schema, tt.token.Token)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, enc)
				if tt.errorMsg != "" {
					require.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, enc)
				require.NotNil(t, enc.enc)
			}
		})
	}
}

func TestOcfEncoder_Append(t *testing.T) {
	// Create a caveat context for testing
	caveatContext, err := structpb.NewStruct(map[string]any{
		"intVal":    42,
		"stringVal": "test",
		"boolVal":   true,
		"nullVal":   nil,
		"nested":    map[string]any{"inner": "value"},
	})
	require.NoError(t, err)

	tests := []struct {
		name         string
		relationship *v1.Relationship
		cursor       string
		expectError  bool
	}{
		{
			name: "simple relationship",
			relationship: &v1.Relationship{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc1",
				},
				Relation: "viewer",
				Subject: &v1.SubjectReference{
					Object: &v1.ObjectReference{
						ObjectType: "user",
						ObjectId:   "alice",
					},
				},
			},
			cursor:      "",
			expectError: false,
		},
		{
			name: "relationship with subject relation",
			relationship: &v1.Relationship{
				Resource: &v1.ObjectReference{
					ObjectType: "folder",
					ObjectId:   "folder1",
				},
				Relation: "viewer",
				Subject: &v1.SubjectReference{
					Object: &v1.ObjectReference{
						ObjectType: "group",
						ObjectId:   "engineers",
					},
					OptionalRelation: "member",
				},
			},
			cursor:      "cursor123",
			expectError: false,
		},
		{
			name: "relationship with caveat",
			relationship: &v1.Relationship{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc2",
				},
				Relation: "viewer",
				Subject: &v1.SubjectReference{
					Object: &v1.ObjectReference{
						ObjectType: "user",
						ObjectId:   "bob",
					},
				},
				OptionalCaveat: &v1.ContextualizedCaveat{
					CaveatName: "time_restricted",
					Context:    caveatContext,
				},
			},
			cursor:      "cursor456",
			expectError: false,
		},
		{
			name: "relationship with caveat name only",
			relationship: &v1.Relationship{
				Resource: &v1.ObjectReference{
					ObjectType: "file",
					ObjectId:   "file1",
				},
				Relation: "editor",
				Subject: &v1.SubjectReference{
					Object: &v1.ObjectReference{
						ObjectType: "user",
						ObjectId:   "charlie",
					},
				},
				OptionalCaveat: &v1.ContextualizedCaveat{
					CaveatName: "ip_restricted",
				},
			},
			cursor:      "",
			expectError: false,
		},
		{
			name: "relationship with empty strings",
			relationship: &v1.Relationship{
				Resource: &v1.ObjectReference{},
				Relation: "",
				Subject:  &v1.SubjectReference{Object: &v1.ObjectReference{}},
			},
			cursor:      "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewOcfEncoder(&buf)
			err := enc.WriteSchema("definition test {}", "test")
			require.NoError(t, err)

			err = enc.Append(tt.relationship, tt.cursor)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify data was written to buffer
				require.NotEmpty(t, buf.Bytes())
			}
		})
	}
}

func TestOcfEncoder_AppendNilRelationship(t *testing.T) {
	var buf bytes.Buffer
	enc := NewOcfEncoder(&buf)
	err := enc.WriteSchema("definition test {}", "test")
	require.NoError(t, err)
	defer require.NoError(t, enc.Close())

	// This should panic as expected when accessing nil relationship fields
	require.Panics(t, func() {
		require.NoError(t, enc.Append(nil, ""))
	})
}

func TestOcfEncoder_Close(t *testing.T) {
	tests := []struct {
		name           string
		appendData     bool
		expectError    bool
		expectNonEmpty bool
	}{
		{
			name:           "close empty encoder",
			appendData:     false,
			expectError:    false,
			expectNonEmpty: true, // Schema is always written
		},
		{
			name:           "close encoder with data",
			appendData:     true,
			expectError:    false,
			expectNonEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewOcfEncoder(&buf)
			err := enc.WriteSchema("definition test {}", "test")
			require.NoError(t, err)

			if tt.appendData {
				rel := &v1.Relationship{
					Resource: &v1.ObjectReference{
						ObjectType: "user",
						ObjectId:   "alice",
					},
					Relation: "self",
					Subject: &v1.SubjectReference{
						Object: &v1.ObjectReference{
							ObjectType: "user",
							ObjectId:   "alice",
						},
					},
				}
				err = enc.Append(rel, "")
				require.NoError(t, err)
			}

			err = enc.Close()
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.expectNonEmpty {
					require.NotEmpty(t, buf.Bytes())
				}
			}
		})
	}
}

func TestOcfEncoder_MarkComplete(t *testing.T) {
	var buf bytes.Buffer
	enc := NewOcfEncoder(&buf)
	err := enc.WriteSchema("definition test {}", "test")
	require.NoError(t, err)
	defer require.NoError(t, enc.Close())

	// MarkComplete should not return error or panic
	require.NotPanics(t, func() { enc.MarkComplete() })

	// Should be able to call multiple times
	enc.MarkComplete()
	enc.MarkComplete()
}

func TestOcfEncoder_MultipleOperations(t *testing.T) {
	var buf bytes.Buffer
	enc := NewOcfEncoder(&buf)
	err := enc.WriteSchema("definition user {}\ndefinition document { relation viewer: user }", "test_token")
	require.NoError(t, err)
	defer require.NoError(t, enc.Close())

	// Test multiple appends
	relationships := []*v1.Relationship{
		{
			Resource: &v1.ObjectReference{ObjectType: "document", ObjectId: "doc1"},
			Relation: "viewer",
			Subject:  &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "alice"}},
		},
		{
			Resource: &v1.ObjectReference{ObjectType: "document", ObjectId: "doc2"},
			Relation: "viewer",
			Subject:  &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "bob"}},
		},
		{
			Resource: &v1.ObjectReference{ObjectType: "document", ObjectId: "doc3"},
			Relation: "viewer",
			Subject:  &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "charlie"}},
		},
	}

	cursors := []string{"cursor_a", "cursor_b", "cursor_c"}
	for i, rel := range relationships {
		err = enc.Append(rel, cursors[i])
		require.NoError(t, err)
	}

	enc.MarkComplete()

	// Verify data was written
	require.NotEmpty(t, buf.Bytes())
}

func TestOcfEncoder_InvalidCaveatContext(t *testing.T) {
	var buf bytes.Buffer
	enc := NewOcfEncoder(&buf)
	err := enc.WriteSchema("definition test {}", "test")
	require.NoError(t, err)
	defer require.NoError(t, enc.Close())

	// Create a relationship with invalid caveat context
	rel := &v1.Relationship{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Relation: "viewer",
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: "user",
				ObjectId:   "alice",
			},
		},
		OptionalCaveat: &v1.ContextualizedCaveat{
			CaveatName: "test_caveat",
			// Context is nil, which should be handled gracefully
		},
	}

	// This should not error even with nil context
	err = enc.Append(rel, "")
	require.NoError(t, err)
}

func TestOcfEncoder_1kRels(t *testing.T) {
	var buf bytes.Buffer
	enc := NewOcfEncoder(&buf)
	err := enc.WriteSchema("definition user {}\ndefinition document { relation viewer: user }", "large_test")
	require.NoError(t, err)
	defer require.NoError(t, enc.Close())

	for i := range 1_000 {
		rel := &v1.Relationship{
			Resource: &v1.ObjectReference{
				ObjectType: "document",
				ObjectId:   fmt.Sprintf("doc%d", i),
			},
			Relation: "viewer",
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   fmt.Sprintf("user%d", i),
				},
			},
		}
		err = enc.Append(rel, "")
		require.NoError(t, err)
	}

	require.NotEmpty(t, buf.Bytes())
}

func TestNewFileEncoder(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		filename    string
		schema      string
		token       *v1.ZedToken
		expectError bool
	}{
		{
			name:        "valid file encoder creation",
			filename:    filepath.Join(tempDir, "test1.backup"),
			schema:      "definition user {}",
			token:       &v1.ZedToken{Token: "test_token"},
			expectError: false,
		},
		{
			name:        "stdout file encoder",
			filename:    "-",
			schema:      "definition user {}",
			token:       &v1.ZedToken{Token: "test_token"},
			expectError: false,
		},
		{
			name:        "empty schema file encoder",
			filename:    filepath.Join(tempDir, "test2.backup"),
			schema:      "",
			token:       &v1.ZedToken{Token: "test_token"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, existed, err := NewFileEncoder(tt.filename)
			if !existed && err == nil {
				err = enc.WriteSchema(tt.schema, tt.token.Token)
			}
			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, enc)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, enc)
			require.NotNil(t, enc.OcfEncoder)
			if tt.filename != "-" {
				enc.Close()
				os.Remove(tt.filename)
				os.Remove(tt.filename + ".lock")
			}
		})
	}
}

func TestWithProgress(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{
			name:   "empty prefix",
			prefix: "",
		},
		{
			name:   "user prefix",
			prefix: "user",
		},
		{
			name:   "document prefix",
			prefix: "document",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewOcfEncoder(&buf)
			err := enc.WriteSchema("definition user {}", "test")
			require.NoError(t, err)

			rw := &PrefixFilterer{Prefix: tt.prefix}
			progEnc := WithProgress(WithRewriter(rw, enc))
			require.NotNil(t, progEnc)
			require.NotNil(t, progEnc.startTime)
			require.NotNil(t, progEnc.ticker)
		})
	}
}

func TestProgressRenderingEncoder_Append(t *testing.T) {
	tests := []struct {
		name         string
		prefix       string
		relationship *v1.Relationship
		expectStored bool
		expectError  bool
	}{
		{
			name:   "matching prefix - both user",
			prefix: "user",
			relationship: &v1.Relationship{
				Resource: &v1.ObjectReference{ObjectType: "user", ObjectId: "alice"},
				Relation: "self",
				Subject:  &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "alice"}},
			},
			expectStored: true,
			expectError:  false,
		},
		{
			name:   "non-matching prefix - resource mismatch",
			prefix: "user",
			relationship: &v1.Relationship{
				Resource: &v1.ObjectReference{ObjectType: "document", ObjectId: "doc1"},
				Relation: "viewer",
				Subject:  &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "alice"}},
			},
			expectStored: false,
			expectError:  false,
		},
		{
			name:   "empty prefix - should match everything",
			prefix: "",
			relationship: &v1.Relationship{
				Resource: &v1.ObjectReference{ObjectType: "anything", ObjectId: "anything"},
				Relation: "relation",
				Subject:  &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "anything", ObjectId: "anything"}},
			},
			expectStored: true,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var innerBuf bytes.Buffer
			innerEnc := NewOcfEncoder(&innerBuf)
			err := innerEnc.WriteSchema("definition user {}\ndefinition document { relation viewer: user }", "test")
			require.NoError(t, err)
			defer innerEnc.Close()

			rw := &PrefixFilterer{Prefix: tt.prefix}
			progEnc := WithProgress(WithRewriter(rw, innerEnc))

			initialProcessed := progEnc.relsProcessed
			initialFiltered := rw.skipped

			err = progEnc.Append(tt.relationship, "cursor123")
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				require.Equal(t, initialProcessed+1, progEnc.relsProcessed, "should always increment processed count")

				if tt.expectStored {
					require.Equal(t, initialFiltered, rw.skipped, "should not increment filtered count")
					require.NotEmpty(t, innerBuf.Bytes(), "should have data in buffer")
				} else {
					require.Equal(t, initialFiltered+1, rw.skipped, "should increment filtered count")
				}
			}
		})
	}
}
