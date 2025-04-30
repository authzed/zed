package main

import (
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Build mg.Namespace

// Binary builds the zed binary
func (g Build) Binary() error {
	return sh.RunV("go", "build", "-o", "zed", "./cmd/zed")
}
