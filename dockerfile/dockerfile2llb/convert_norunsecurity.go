//go:build !dfrunsecurity

package dockerfile2llb

import (
	"github.com/btidor/syntax/dockerfile/instructions"
	"github.com/moby/buildkit/client/llb"
)

func dispatchRunSecurity(_ *instructions.RunCommand) (llb.RunOption, error) {
	return nil, nil
}
