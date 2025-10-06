//go:build mage
// +build mage

package main

import (
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Lint mg.Namespace

// All runs all linters
func (g Lint) All() error {
	mg.Deps(g.Go, g.Vulncheck)
	return nil
}

// Go runs golangci-lint
func (g Lint) Go() error {
	return sh.RunV("go", "run", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint", "run", "--fix", "-c", ".golangci.yaml", "./...")
}

// Vulncheck runs vulncheck
func (g Lint) Vulncheck() error {
	return sh.RunV("go", "run", "golang.org/x/vuln/cmd/govulncheck", "./...")
}
