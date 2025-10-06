//go:build mage
// +build mage

package main

import (
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Test mg.Namespace

// Run runs unit tests
func (g Test) Run() error {
	return sh.RunV("go", "test", "-race", "-count=1", "-timeout=10m", "./...")
}

// RunWithCoverage runs unit tests and measures coverage (useful for CI)
func (g Test) RunWithCoverage() error {
	return sh.RunV("go", "test", "-race", "-count=1", "-timeout=10m", "-covermode=atomic", "-coverprofile=coverage.txt", "./...")
}
