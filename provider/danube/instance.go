// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package danube

import (
	"github.com/erigones/godanube/cloudapi"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

type joyentInstance struct {
// J
	machine *cloudapi.VmDetails
	//nics    []cloudapi.VmNicDefinition	DELME
	env     *joyentEnviron
}

var _ instances.Instance = (*joyentInstance)(nil)

func (inst *joyentInstance) Id() instance.Id {
	return instance.Id(inst.machine.Uuid)
}

func (inst *joyentInstance) Status(ctx context.ProviderCallContext) instance.Status {
	instStatus := inst.machine.Status
	var jujuStatus status.Status
	switch instStatus {
	case "notcreated", "deploying", "notready":
		jujuStatus = status.Allocating
	case "running", "running-":
		jujuStatus = status.Running
	case "stopping", "stopped", "stopped-", "frozen", "unreachable":
		jujuStatus = status.Empty
	case "error":
		jujuStatus = status.ProvisioningError
	default:
		jujuStatus = status.Empty
	}
	return instance.Status{
		Status:  jujuStatus,
		Message: instStatus,
	}
}

func (inst *joyentInstance) Addresses(ctx context.ProviderCallContext) (network.ProviderAddresses, error) {
	machineNics, err := inst.env.compute.cloudapi.GetMachineNics(inst.machine.Uuid)
	if err != nil {
		return nil, err
	}

	addresses := make([]network.ProviderAddress, 0, len(machineNics))
	for _, nic := range machineNics {
		address := network.NewProviderAddress(nic.Ip)
		if nic.Primary == true {
			address.Scope = network.ScopePublic
		} else {
			address.Scope = network.ScopeCloudLocal
		}
		addresses = append(addresses, address)
	}

	logger.Debugf("Returning %d machine addresses", len(addresses))
	return addresses, nil
}
