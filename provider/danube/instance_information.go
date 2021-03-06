// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package danube

import (
	//"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	//"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

//var _ environs.InstanceTypesFetcher = (*joyentEnviron)(nil)

// InstanceTypes implements InstanceTypesFetcher
func (env *joyentEnviron) InstanceTypes(ctx context.ProviderCallContext, c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
    //J XXX DELME
    logger.Debugf("J XXX InstanceTypes() was called with constraints %+v and ctx %+v", c, ctx)
    return instances.InstanceTypesWithCostMetadata{}, nil

    /*
	iTypes, err := env.listInstanceTypes()
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	iTypes, err = instances.MatchingInstanceTypes(iTypes, "", c)
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	return instances.InstanceTypesWithCostMetadata{InstanceTypes: iTypes}, nil
    */
}
