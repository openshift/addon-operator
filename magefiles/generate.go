//go:build mage
// +build mage

package main

import (
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Generate mg.Namespace

func (Generate) All() {
	mg.Deps(
		Generate.docs,
	)
}

func (Generate) docs() error {
	mg.Deps(Build.Docgen)

	return sh.Run("./hack/docgen.sh")
}
