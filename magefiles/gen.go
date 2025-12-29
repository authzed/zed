//go:build mage
// +build mage

package main

import (
	"os"

	"github.com/jzelinskie/cobrautil/v2/cobrazerolog"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"

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
