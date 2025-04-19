//go:build mage
// +build mage

package main

import (
	"os"

	"github.com/jzelinskie/cobrautil/v2/cobrazerolog"
	"github.com/magefile/mage/mg"

	"github.com/authzed/zed/internal/cmd"
)

type Gen mg.Namespace

// All Run all generators in parallel
func (g Gen) All() error {
	mg.Deps(g.Docs)
	return nil
}

// Generate markdown files for zed
func (Gen) Docs() error {
	targetDir := "docs"

	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return err
	}

	rootCmd := cmd.InitialiseRootCmd(cobrazerolog.New())

	return GenCustomMarkdownTree(rootCmd, targetDir)
}
