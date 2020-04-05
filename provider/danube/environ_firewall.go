// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package danube

import (
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

// OpenPorts is part of the environs.Firewaller interface.
func (*joyentEnviron) OpenPorts(rules []network.IngressRule) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// ClosePorts is part of the environs.Firewaller interface.
func (*joyentEnviron) ClosePorts(rules []network.IngressRule) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// IngressPorts is part of the environs.Firewaller interface.
func (*joyentEnviron) IngressRules() ([]network.IngressRule, error) {
	return nil, errors.Trace(errors.NotSupportedf("Ports"))
}
