// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package danube

import (
	"fmt"
	"strings"
	"sync"

	"github.com/erigones/godanube/cloudapi"
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/common"
)

// This file contains the core of the Joyent Environ implementation.

type joyentEnviron struct {
	name    string
	cloud   environs.CloudSpec
	compute *joyentCompute

	lock sync.Mutex // protects ecfg
	ecfg *environConfig
}

// newEnviron create a new Joyent environ instance from config.
func newEnviron(cloud environs.CloudSpec, cfg *config.Config) (*joyentEnviron, error) {
	env := &joyentEnviron{
		name:  cfg.Name(),
		cloud: cloud,
	}
	if err := env.SetConfig(cfg); err != nil {
		return nil, err
	}
	var err error
	env.compute, err = newCompute(cloud)
	if err != nil {
		return nil, err
	}
	return env, nil
}

//var _ environs.Environ = (*maasEnviron)(nil)	//J XXX delme
var _ environs.Networking = (*joyentEnviron)(nil)

func (env *joyentEnviron) SetName(envName string) {
	env.name = envName
}

func (*joyentEnviron) Provider() environs.EnvironProvider {
	return providerInstance
}

// PrecheckInstance is defined on the environs.InstancePrechecker interface.
func (env *joyentEnviron) PrecheckInstance(ctx context.ProviderCallContext, args environs.PrecheckInstanceParams) error {
	return nil

	// J do we want to check node placement?
	if args.Placement != "" {
		return fmt.Errorf("unknown placement directive: %s", args.Placement)
	}

	/* J XXX DELME
	if !args.Constraints.HasInstanceType() {
		return nil
	}
	// Constraint has an instance-type constraint so let's see if it is valid.
	instanceTypes, err := env.listInstanceTypes()
	if err != nil {
		return err
	}
	for _, instanceType := range instanceTypes {
		if instanceType.Name == *args.Constraints.InstanceType {
			return nil
		}
	}
	return fmt.Errorf("invalid Joyent instance %q specified", *args.Constraints.InstanceType)
	*/
	return nil
}

func (env *joyentEnviron) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return err
	}
	env.ecfg = ecfg
	return nil
}

func (env *joyentEnviron) Config() *config.Config {
	return env.Ecfg().Config
}

// Create is part of the Environ interface.
func (env *joyentEnviron) Create(context.ProviderCallContext, environs.CreateParams) error {
	if err := verifyCredentials(env); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (env *joyentEnviron) PrepareForBootstrap(ctx environs.BootstrapContext, controllerName string) error {
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (env *joyentEnviron) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	return common.Bootstrap(ctx, env, callCtx, args)
}

func (env *joyentEnviron) ControllerInstances(ctx context.ProviderCallContext, controllerUUID string) ([]instance.Id, error) {
	instanceIds := []instance.Id{}

	filter := cloudapi.VmDetails {
		Tags: []string{
			makeDanubeTag("group", "juju"),
			makeDanubeTag(tags.JujuModel, controllerUUID),
			makeDanubeTag(tags.JujuIsController, "true"),
		},
	}

	vmDetails, err := env.compute.cloudapi.ListMachinesFilteredFull(filter)
	if err != nil || len(vmDetails) == 0 {
		return nil, environs.ErrNotBootstrapped
	}

	for _, m := range vmDetails {
		// status can be "running" or "running-" (if the VM changes are being applied)
		if strings.EqualFold(m.Status, "deploying") || strings.HasPrefix(m.Status, "running") {
			copy := m
			ji := &joyentInstance{machine: &copy, env: env}
			instanceIds = append(instanceIds, ji.Id())
		}
	}

	return instanceIds, nil
}

// AdoptResources is part of the Environ interface.
func (env *joyentEnviron) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	// This provider doesn't track instance -> controller.
	return nil
}

func (env *joyentEnviron) Destroy(ctx context.ProviderCallContext) error {
	return errors.Trace(common.Destroy(env, ctx))
}

// DestroyController implements the Environ interface.
func (env *joyentEnviron) DestroyController(ctx context.ProviderCallContext, controllerUUID string) error {
	// TODO(wallyworld): destroy hosted model resources
	return env.Destroy(ctx)
}

func (env *joyentEnviron) Ecfg() *environConfig {
	env.lock.Lock()
	defer env.lock.Unlock()
	return env.ecfg
}

// MetadataLookupParams returns parameters which are used to query simplestreams metadata.
func (env *joyentEnviron) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
// J XXX
	if region == "" {
		region = env.cloud.Region
	}
	return &simplestreams.MetadataLookupParams{
		Series:   config.PreferredSeries(env.Ecfg()),
		//J XXX neviem co robim
		//Region:   region,
		//Endpoint: env.cloud.Endpoint,
	}, nil
}

// Region is specified in the HasRegion interface.
func (env *joyentEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   "worldwide",
		Endpoint: "danubecloud",
		//Region:   "danubecloud",
		//Region:   env.cloud.Region,
		//Endpoint: env.cloud.Endpoint,
	}, nil
}
