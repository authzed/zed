//go:build mage
// +build mage

package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/jzelinskie/cobrautil/v2/cobrazerolog"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/cmd"
)

type Gen mg.Namespace

// All Run all generators in parallel
func (g Gen) All() error {
	mg.Deps(g.Docs, g.mocks)
	return nil
}

// mocks Generate mocks using go generate
func (g Gen) mocks() error {
	return sh.RunV("go", "generate", "./...")
}

// Docs Generate documentation in markdown format
func (g Gen) Docs() error {
	targetDir := "docs"

	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return err
	}

	rootCmd := cmd.InitialiseRootCmd(cobrazerolog.New())

	return GenCustomMarkdownTree(rootCmd, targetDir)
}

// DocsForPublish generates a markdown file for publishing in the docs website.
func (g Gen) DocsForPublish() error {
	if err := g.Docs(); err != nil {
		return err
	}

	return sh.RunV("bash", "-c", "cat docs/getting-started.md <(echo -e '\\n') docs/zed.md > docs/merged.md")
}

// Completions Generate shell completion scripts for bash, zsh, and fish
func (g Gen) Completions() error {
	targetDir := "completions"

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	rootCmd := cmd.InitialiseRootCmd(cobrazerolog.New())

	generators := []struct {
		shell    string
		generate func(*cobra.Command, io.Writer) error
	}{
		{"bash", func(c *cobra.Command, w io.Writer) error { return c.GenBashCompletionV2(w, true) }},
		{"zsh", func(c *cobra.Command, w io.Writer) error { return c.GenZshCompletion(w) }},
		{"fish", func(c *cobra.Command, w io.Writer) error { return c.GenFishCompletion(w, true) }},
	}

	for _, gen := range generators {
		path := filepath.Join(targetDir, "zed."+gen.shell)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := gen.generate(rootCmd, f); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}
