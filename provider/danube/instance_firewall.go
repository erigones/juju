// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package danube

import (
	"github.com/juju/errors"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/provider/common"
)

/* XXX DELME
func (inst *joyentInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, ports []network.IngressRule) error {
	return nil
}

func (inst *joyentInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, ports []network.IngressRule) error {
	return nil
}

func (inst *joyentInstance) IngressRules(ctx context.ProviderCallContext, machineId string) ([]network.IngressRule, error) {
	return getRules(inst.env.Config().Name(), fwRules)
}*/


// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *joyentInstance) OpenPorts(ctx context.ProviderCallContext, machineID string, rules []network.IngressRule) error {
    logger.Debugf("Opening port(s) on machine '%s' with rules %+v", machineID, rules)
	return inst.changeIngressRules(ctx, true, rules)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *joyentInstance) ClosePorts(ctx context.ProviderCallContext, machineID string, rules []network.IngressRule) error {
    logger.Debugf("Closing port(s) on machine '%s' with rules %+v", machineID, rules)
	return inst.changeIngressRules(ctx, false, rules)
}

// IngressRules returns the set of ports open on the instance, which
// should have been started with the given machine id.
func (inst *joyentInstance) IngressRules(ctx context.ProviderCallContext, machineID string) ([]network.IngressRule, error) {
    logger.Debugf("Listing firewall rules on machine '%s'")
	_, client, err := inst.getInstanceConfigurator(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.FindIngressRules()
}

func (inst *joyentInstance) changeIngressRules(ctx context.ProviderCallContext, insert bool, rules []network.IngressRule) error {
	addresses, client, err := inst.getInstanceConfigurator(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	for _, addr := range addresses {
        //J XXX uncomment
		//if addr.Type == corenetwork.IPv6Address || addr.Scope != corenetwork.ScopePublic {
		if addr.Type == corenetwork.IPv6Address {
			// TODO(axw) support firewalling IPv6
			continue
		}
		if err := client.ChangeIngressRules(addr.Value, insert, rules); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (inst *joyentInstance) getInstanceConfigurator(
	ctx context.ProviderCallContext,
) ([]corenetwork.ProviderAddress, common.InstanceConfigurator, error) {
	addresses, err := inst.Addresses(ctx)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var localAddr string
    if len(addresses) == 1 {
        // there's only one address, we will connect to it
        // regardless its type
        localAddr = addresses[0].Value
    } else {
        for _, addr := range addresses {
            if addr.Scope == corenetwork.ScopeCloudLocal {
                localAddr = addr.Value
                break
            }
        }
    }

    localAddr = inst.machine.Ips[0]    //J XXX DELME
    logger.Debugf("Creating SshInstanceConfigurator over IP %s", localAddr)
	client := common.NewSshInstanceConfigurator(localAddr)
	return addresses, client, err
}
