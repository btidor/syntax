//go:build !dfrundevice

package dockerfile2llb

import (
	"github.com/btidor/syntax/dockerfile/instructions"
	"github.com/moby/buildkit/client/llb"
	"github.com/pkg/errors"
)

func dispatchRunDevices(c *instructions.RunCommand) ([]llb.RunOption, error) {
	if len(instructions.GetDevices(c)) > 0 {
		return nil, errors.Errorf("device feature is only supported in Dockerfile frontend 1.14.0-labs or later")
	}
	return nil, nil
}
