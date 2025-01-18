//go:build mage
// +build mage

package main

import (
	"os"

	"github.com/authzed/zed/internal/cmd"
	"github.com/jzelinskie/cobrautil/v2/cobrazerolog"
	"github.com/magefile/mage/mg"
	"github.com/spf13/cobra/doc"
)

type Gen mg.Namespace

// All Run all generators in parallel
func (g Gen) All() error {
	mg.Deps(g.Docs)
	return nil
}

// Generate markdown files for zed
func (Gen) Docs() error {
	targetDir := "../docs"

	err := os.MkdirAll("../docs", os.ModePerm)
	if err != nil {
		return err
	}

	rootCmd := cmd.InitialiseRootCmd(cobrazerolog.New())
	return doc.GenMarkdownTree(rootCmd, targetDir)
}
