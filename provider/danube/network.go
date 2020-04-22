// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package danube

import (
	"net"
	"fmt"

	"gopkg.in/juju/names.v3"
	"github.com/juju/errors"
	"github.com/erigones/godanube/cloudapi"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
)

// SupportsSpaces is specified on environs.Networking.
func (env *joyentEnviron) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
	return true, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (env *joyentEnviron) SupportsSpaceDiscovery(ctx context.ProviderCallContext) (bool, error) {
	return true, nil
}

// SupportsContainerAddresses is specified on environs.Networking.
func (env *joyentEnviron) SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error) {
	return false, nil
}

// Spaces returns all the spaces, that have subnets, known to the provider.
// Space name is not filled in as the provider doesn't know the juju name for
// the space.
func (env *joyentEnviron) Spaces(ctx context.ProviderCallContext) ([]corenetwork.SpaceInfo, error) {
	var result []corenetwork.SpaceInfo
    availableNets, err := env.compute.cloudapi.GetAttachedNetworks()
	if err != nil {
		return result, err
	}

	for _, space := range availableNets {
		if space.Dhcp_passthrough == true {
			// don't include networks with externally managed IP addresses
			continue
		}

		outSpace := corenetwork.SpaceInfo{
			// space.Name is unique
			Name:       corenetwork.SpaceName(space.Name),
			//ID:         space.Uuid,
			ID:         space.Name,
			ProviderId: corenetwork.Id(space.Name),
			Subnets:    []corenetwork.SubnetInfo{
				{
					CIDR:            fmt.Sprintf("%s/%d", space.Network, ipMaskToPrefix(space.Netmask)),
					ProviderId:      corenetwork.Id(space.Name),
					//ProviderSpaceId: corenetwork.Id(space.Uuid),
					ProviderSpaceId: corenetwork.Id(space.Name),
					// SubnetInfo doesn't support vxlans so we have to reuse some field
					ProviderNetworkId: corenetwork.Id(fmt.Sprintf("%s_%d", space.Name, space.Vxlan_id)),
					VLANTag:         space.Vlan_id,
					//SpaceID:         space.Uuid,
					SpaceID:         space.Name,
					SpaceName:       space.Name,
				},
			},
		}
		result = append(result, outSpace)
	}

	return result, nil
}

func ipMaskToPrefix(netmask string) int {
	prefix, _ := net.IPMask(net.ParseIP(netmask).To4()).Size()
	return prefix
}

// Subnets returns basic information about the specified subnets known
// by the provider for the specified instance. subnetIds must not be
// empty. Implements NetworkingEnviron.Subnets.
func (env *joyentEnviron) Subnets(
	ctx context.ProviderCallContext, instId instance.Id, subnetIds []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	allSpaces, err := env.Spaces(ctx)
	if err != nil {
		return nil, err
	}

    machineNics, err := env.compute.cloudapi.GetMachineNics(string(instId))
	if err != nil {
		return nil, err
	}

	var instanceNets []corenetwork.SubnetInfo
	var filteredNets []corenetwork.SubnetInfo

	for _, nic := range machineNics {
		for _, space := range allSpaces {
			subnet := space.Subnets[0]
			if nic.Net == subnet.SpaceName {
				// network is attached to the machine
				instanceNets = append(instanceNets, subnet)
				if subnet.ProviderId == corenetwork.Id(nic.Net) {
					// network is also in subnetIds list
					filteredNets = append(filteredNets, subnet)
				}
			}
		}
	}

	logger.Debugf("Requested Subnets(); found: %d; filtered: %d", len(instanceNets), len(filteredNets))

	return filteredNets, nil
}

// ProviderSpaceInfo implements environs.NetworkingEnviron.
func (*joyentEnviron) ProviderSpaceInfo(
	ctx context.ProviderCallContext, space *corenetwork.SpaceInfo,
) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// AreSpacesRoutable implements environs.NetworkingEnviron.
func (*joyentEnviron) AreSpacesRoutable(ctx context.ProviderCallContext, space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SSHAddresses implements environs.SSHAddresses.
func (*joyentEnviron) SSHAddresses(ctx context.ProviderCallContext, addresses corenetwork.SpaceAddresses) (corenetwork.SpaceAddresses, error) {
	return addresses, nil
}

// SuperSubnets implements environs.SuperSubnets
func (*joyentEnviron) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}

func (env *joyentEnviron) AllocateContainerAddresses(ctx context.ProviderCallContext, hostInstanceID instance.Id, containerTag names.MachineTag, preparedInfo []corenetwork.InterfaceInfo) ([]corenetwork.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("allocate container addresses")
}

func (env *joyentEnviron) ReleaseContainerAddresses(ctx context.ProviderCallContext, interfaces []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("allocate container addresses")
}

// NetworkInterfaces implements Environ.NetworkInterfaces.
func (env *joyentEnviron) NetworkInterfaces(ctx context.ProviderCallContext, ids []instance.Id) ([][]corenetwork.InterfaceInfo, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	// Fetch instance information for the IDs we are interested in.
	insts, err := env.Instances(ctx, ids)

	partialInfo := err == environs.ErrPartialInstances
	if err != nil && err != environs.ErrPartialInstances {
		if err == environs.ErrNoInstances {
			return nil, err
		}
		return nil, err
	}

	infos := make([][]corenetwork.InterfaceInfo, len(ids))

	for idx, inst := range insts {
		if inst == nil {
			continue // unknown instance ID
		}

		nics, err := env.compute.cloudapi.GetMachineNics(string(inst.Id()))
		if err != nil {
			return nil, err
		}

		ifaceList := nicsToNetworkInfo(nics)

		infos[idx] = ifaceList
	}

	if partialInfo {
		err = environs.ErrPartialInstances
	}
	return infos, err
}


func nicsToNetworkInfo(nics []cloudapi.VmNicDefinition) ([]corenetwork.InterfaceInfo) {
	ifaceList := make([]corenetwork.InterfaceInfo, 0, len(nics))
	for index, nic := range nics {
		nicInfo := corenetwork.InterfaceInfo{
			// this struct wants primary interface on index 0
			// but the real primary interface is determined by nic.Primary.
			// We could swap primary interface with the first one if neccessary but it would not represent the interface ordering inside the machine
			DeviceIndex:         index,
			MACAddress:          nic.Mac,
			ProviderId:          corenetwork.Id(nic.Net),
			//ProviderSpaceId:     corenetwork.Id(space.Uuid),
			ProviderSpaceId:     corenetwork.Id(nic.Net),
			CIDR:                fmt.Sprintf("%s/%d", nic.Ip, ipMaskToPrefix(nic.Netmask)),
			//InterfaceName:       fmt.Sprintf("eth%d", index),	// not guaranteed
			//InterfaceName:       fmt.Sprintf("net%d", index),	// not guaranteed
			InterfaceType:       corenetwork.EthernetInterface,
			MTU:                 nic.Mtu,
		}
		ifaceList = append(ifaceList, nicInfo)
	}
	return ifaceList
}
