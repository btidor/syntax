package dockerfile2llb

import (
	"github.com/btidor/syntax/dockerfile/instructions"
)

const PackageStepCount = 1

type PackageInvocation struct{}

func NewPackageInvocation(d *dispatchState, c *instructions.PackageCommand, dopt dispatchOpt) *PackageInvocation {
	panic("not implemented")
}

func (i *PackageInvocation) Dispatch() error {
	panic("not implemented")
}
