// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package danube

import (
	"strings"
	"sync"
	"time"
    "fmt"

	"github.com/erigones/godanube/client"
	"github.com/erigones/godanube/cloudapi"
	dcerrors "github.com/erigones/godanube/errors"
	"github.com/juju/errors"
	"github.com/juju/utils/arch"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/tools"
)

var (
	vTypeSmartmachine   = "smartmachine"
	// J XXX
	vTypeVirtualmachine = "kvm"	// j TODO add to simplestreams params
	defaultCpuCores     = uint64(1)
	defaultMem          = uint64(1024)

    maxRepoRefreshAge    = time.Hour * 24 // when to issue remote img repo refresh
    //remoteDanubeRepoName = "danubecloud"
    remoteDanubeRepoName = "images.joyent.com"		// XXX Delme
    dcGithubRepo = "https://github.com/erigones/esdc-ce"
)

type joyentCompute struct {
	cloudapi *cloudapi.Client
}

func newCompute(cloud environs.CloudSpec) (*joyentCompute, error) {
	creds, err := credentials(cloud)
	if err != nil {
		return nil, err
	}

	client := client.NewClient(creds, cloudapi.DefaultAPIVersion, newGoLogger())

	if logger.EffectiveLogLevel() == loggo.DEBUG {
		client.SetTrace(true)
	}

	return &joyentCompute{cloudapi: cloudapi.New(client)}, nil
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	// J XXX
	constraints.Tags,
	// J XXX
	constraints.VirtType,
}

// ConstraintsValidator is defined on the Environs interface.
func (env *joyentEnviron) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
    /*
	packages, err := env.compute.cloudapi.ListPackages(nil)
	if err != nil {
		return nil, err
	}
	instTypeNames := make([]string, len(packages))
	for i, pkg := range packages {
		instTypeNames[i] = pkg.Name
	}
	validator.RegisterVocabulary(constraints.InstanceType, instTypeNames)
    */
	validator.RegisterVocabulary(constraints.InstanceType, []string{"default"})
	return validator, nil
}

// MaintainInstance is specified in the InstanceBroker interface.
func (*joyentEnviron) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	return nil
}

func (env *joyentEnviron) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	series := args.Tools.OneSeries()
	arches := args.Tools.Arches()

	spec, err := env.FindInstanceSpec(&instances.InstanceConstraint{
		Region:      env.cloud.Region,
		Series:      series,
		Arches:      arches,
		Constraints: args.Constraints,
	}, args.ImageMetadata)
	if err != nil {
		return nil, err
	}
	tools, err := args.Tools.Match(tools.Filter{Arch: spec.Image.Arch})
	if err != nil {
		return nil, errors.Errorf("chosen architecture %v not present in %v", spec.Image.Arch, arches)
	}

	if err := args.InstanceConfig.SetTools(tools); err != nil {
		return nil, errors.Trace(err)
	}

	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.Config()); err != nil {
		return nil, err
	}

	// This is a hack that ensures that instances can communicate over
	// the internal network. Joyent sometimes gives instances
	// different 10.x.x.x/21 networks and adding this route allows
	// them to talk despite this. See:
	// https://bugs.launchpad.net/juju-core/+bug/1401130
	cloudcfg, err := cloudinit.New(args.InstanceConfig.Series)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create cloudinit template")
	}
	/*
	ifupScript := `
#!/bin/bash

# These guards help to ensure that this hack only runs if Joyent's
# internal network still works as it does at time of writing.
[ "$IFACE" == "eth1" ] || [ "$IFACE" == "--all" ] || exit 0
/sbin/ip -4 --oneline addr show dev eth1 | fgrep --quiet " inet 10." || exit 0

/sbin/ip route add 10.0.0.0/8 dev eth1
`[1:]
	cloudcfg.AddBootTextFile("/etc/network/if-up.d/joyent", ifupScript, 0755)
	*/
	userData, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, JoyentRenderer{})
	if err != nil {
		return nil, errors.Annotate(err, "cannot make user data")
	}
	logger.Debugf("joyent user data: %d bytes", len(userData))

	instanceTags := make(map[string]string)
	for tag, value := range args.InstanceConfig.Tags {
		instanceTags[tag] = value
	}
	instanceTags["group"] = "juju"
	instanceTags["model"] = env.Config().Name()

	args.InstanceConfig.Tags = instanceTags
	logger.Debugf("Now tags are:  %+v", args.InstanceConfig.Tags)

    instName := fmt.Sprintf("juju-%s-%s", env.Config().Name(), args.InstanceConfig.MachineId)
    instTags := toDanubeTags(args.InstanceConfig.Tags)
    instMdata := map[string]string{"cloud-init:user-data": string(userData)}

    imgName, err := env.ensureImagePresent(spec.Image.Id)
	if err != nil {
		return nil, err
	}

	createMachineOpts := cloudapi.CreateMachineOpts {
		Vm: cloudapi.MachineDefinition {
			Name:       instName,
			//Alias: "fullvm",
			//DnsDomain: "lan",
			Vcpus:      int(spec.InstanceType.CpuCores),
			Ram:        int(spec.InstanceType.Mem),
			Tags:       instTags,
			Mdata:      instMdata,
		},
		Disks: []cloudapi.VmDiskDefinition {
			{
                // If disk size was not requested, RootDisk is zero.
                // That means the resulting json will not contain disk size request.
				Size:   int(spec.InstanceType.RootDisk),
				Image:  *imgName,
			},
		},
	}

	if !args.Constraints.HasSpaces() {
		createMachineOpts.Nics = []cloudapi.VmNicDefinition {
			{
				Net:    "mynet",
			},
		}
	} else {
		createMachineOpts.Nics = []cloudapi.VmNicDefinition {
			{
				Net:    "admin",
			},
		}
	}

    logger.Debugf("Creating new machine with following parameters: %+v", createMachineOpts)

	var machine *cloudapi.MachineDefinition
	machine, err = env.compute.cloudapi.CreateMachine(createMachineOpts)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create instances")
	}
	machineId := machine.Uuid

	logger.Infof("provisioning instance %q", machineId)
	logger.Infof("machine created with tags %+v", machine.Tags)

	machineInfo, err := env.compute.cloudapi.GetMachine(machineId)
	if err != nil || !strings.EqualFold(machineInfo.Status, "running") {
		return nil, errors.Annotate(err, "cannot start instances")
	}
	/*
    machineNics, err := env.compute.cloudapi.GetMachineNics(machineId)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start instances")
	}
	DELME XXX
	*/

	logger.Infof("started instance %q", machineId)

	inst := &joyentInstance{
		machine: machineInfo,
        //nics:    machineNics,	DELME
		env:     env,
	}

    /*
    arch := "amd64"
    mem := 1024
    cpus := uint(1)
    */
	disk64 := uint64(machineInfo.Disk)
	hc := instance.HardwareCharacteristics{
		Arch:     &spec.Image.Arch,
		Mem:      &spec.InstanceType.Mem,
		CpuCores: &spec.InstanceType.CpuCores,
		CpuPower: spec.InstanceType.CpuPower,
		RootDisk: &disk64,
	}

	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: &hc,
	}, nil
}

// AllInstances implements environs.InstanceBroker.
func (env *joyentEnviron) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return env.filteredInstances(ctx, "deploying", "running", "running-", "stopping", "stopping-", "stopped", "notready", "notcreated", "frozen", "error")
}

// AllRunningInstances implements environs.InstanceBroker.
func (env *joyentEnviron) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return env.filteredInstances(ctx, "deploying", "running", "running-", "notready")
}

func makeDanubeTag(key, val string) string {
	return key + ":" + val
}

// Danube tags are not key/value pairs, just simple strings
// therefore we have to do a conversion
func toDanubeTags(tags map[string]string) []string {
	var ret []string
	for key, val := range tags {
		ret = append(ret, makeDanubeTag(key, val))
	}
	return ret
}

// AllRunningInstances implements environs.InstanceBroker.
func (env *joyentEnviron) filteredInstances(ctx context.ProviderCallContext, statusFilters ...string) ([]instances.Instance, error) {
	instances := []instances.Instance{}

	filter := cloudapi.VmDetails {
		Tags: []string{
			makeDanubeTag("group", "juju"),
			makeDanubeTag(tags.JujuModel, env.Config().UUID()),
		},
	}

	machines, err := env.compute.cloudapi.ListMachinesFilteredFull(filter)
	if err != nil {
		return nil, errors.Annotate(err, "cannot retrieve instances")
	}

	match := func(current string) bool {
		for _, one := range statusFilters {
			if strings.EqualFold(current, one) {
				return true
			}
		}
		return false
	}

	for _, m := range machines {
		if len(statusFilters) == 0 || match(m.Status) {
			/*
            machineNics, err := env.compute.cloudapi.GetMachineNics(m.Uuid)
            if err != nil {
                return nil, errors.Annotate(err, "cannot retrieve instance nics info")
            }
			DELME XXX
            instances = append(instances, &joyentInstance{machine: &m, nics: machineNics, env: env})
			*/
            instances = append(instances, &joyentInstance{machine: &m, env: env})
		}
	}

	return instances, nil
}

func (env *joyentEnviron) Instances(ctx context.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	logger.Debugf("Looking for instances %q", ids)

	instances := make([]instances.Instance, len(ids))
	found := 0

	allInstances, err := env.AllRunningInstances(ctx)
	if err != nil {
		return nil, err
	}

	for i, id := range ids {
		for _, instance := range allInstances {
			if instance.Id() == id {
				instances[i] = instance
				found++
			}
		}
	}

	logger.Debugf("Found %d instances %q", found, instances)

	if found == 0 {
		return nil, environs.ErrNoInstances
	} else if found < len(ids) {
		return instances, environs.ErrPartialInstances
	}

	return instances, nil
}

func (env *joyentEnviron) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
// J
	// Remove all the instances in parallel so that we incur less round-trips.
	var wg sync.WaitGroup
	//var err error
	wg.Add(len(ids))
	errc := make(chan error, len(ids))
	for _, id := range ids {
		id := id // copy to new free var for closure
		go func() {
			defer wg.Done()
			if err := env.stopInstance(string(id)); err != nil {
				errc <- err
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errc:
		return errors.Annotate(err, "cannot stop all instances")
	default:
	}
	return nil
}

func (env *joyentEnviron) stopInstance(id string) error {
// J
err := env.compute.cloudapi.DeleteMachine(id, true)
	if err != nil {
		return errors.Annotatef(err, "cannot delete instance %v", id)
	}
	return nil
}

func (env *joyentEnviron) pollMachineState(machineId, state string) bool {
// J
	actualState, err := env.compute.cloudapi.GetMachineState(machineId)
	if err != nil {
		return false
	}
	return strings.EqualFold(*actualState, state)
}

//J XXX DELME
/*
func (env *joyentEnviron) listInstanceTypes() ([]instances.InstanceType, error) {
	packages, err := env.compute.cloudapi.ListPackages(nil)
	if err != nil {
		return nil, err
	}
	allInstanceTypes := []instances.InstanceType{}
	for _, pkg := range packages {
		// ListPackages does not include the virt type of the package.
		// However, Joyent says the smart packages have zero VCPUs.
		var virtType *string
		if pkg.VCPUs > 0 {
			virtType = &vTypeVirtualmachine
		} else {
			virtType = &vTypeSmartmachine
		}
		instanceType := instances.InstanceType{
			Id:       pkg.Id,
			Name:     pkg.Name,
			Arches:   []string{arch.AMD64},
			Mem:      uint64(pkg.Memory),
			CpuCores: uint64(pkg.VCPUs),
			RootDisk: uint64(pkg.Disk * 1024),
			VirtType: virtType,
		}
		allInstanceTypes = append(allInstanceTypes, instanceType)
	}

	return allInstanceTypes, nil
}*/

// FindInstanceSpec returns an InstanceSpec satisfying the supplied instanceConstraint.
func (env *joyentEnviron) FindInstanceSpec(
	ic *instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
) (*instances.InstanceSpec, error) {
	// Require at least one VCPU
	if ic.Constraints.CpuCores == nil {
		ic.Constraints.CpuCores = &defaultCpuCores
	}
	// Memory amount is a required parameter
	if ic.Constraints.Mem == nil {
		ic.Constraints.Mem = &defaultMem
	}
    /* DELME
	allInstanceTypes, err := env.listInstanceTypes()
	if err != nil {
		return nil, err
	}*/

    //spec := instanceSpecFromConstraints(ic)
    //J XXX TODO images!!!
    instTypeList := []instances.InstanceType{
        instanceTypeFromConstraints(ic),
    }

	images := instances.ImageMetadataToImages(imageMetadata)
	spec, err := instances.FindInstanceSpec(images, ic, instTypeList)
	if err != nil {
		return nil, err
	}

	return spec, nil
}

func instanceTypeFromConstraints(ic *instances.InstanceConstraint) instances.InstanceType {
    var rootDiskSize uint64

    if ic.Constraints.RootDisk != nil {
        rootDiskSize = *ic.Constraints.RootDisk
    } else {
        // zero means default disk size
        rootDiskSize = 0
    }

    instanceType := instances.InstanceType{
        Id:       "default",
        Name:     "default",
        Arches:   []string{arch.AMD64},
        Mem:      *ic.Constraints.Mem,
        CpuCores: *ic.Constraints.CpuCores,
        RootDisk: rootDiskSize,
        //RootDisk: uint64(pkg.Disk * 1024),
        VirtType: &vTypeVirtualmachine,
    }
    return instanceType
}

/*
func instanceSpecFromConstraints(ic *instances.InstanceConstraint) *instances.InstanceSpec {
    // create one exactly matching instance
	//allInstanceTypes := []instances.InstanceType{}
    //allInstanceTypes = append(allInstanceTypes, instanceType)
    //return allInstanceTypes

    return &instances.InstanceSpec{
        InstanceType: instanceTypeFromConstraints(ic),
        Image:        instances.Image{
            Id:         "ubuntu-certified-18.04",
            VirtType:   vTypeVirtualmachine,
            Arch:       arch.AMD64,
        },
        //order:        0,
    }
}
*/

func isInList(requested string, list []string) bool {
    for _, item := range list {
        if requested == item {
            return true
        }
    }
    return false
}

/**
this function does several things:
- find the image in the Danube Cloud installation
- convert image uuid to image name (because the image itself can be saved under arbitrary name but the uuid stays)
- if the image is not found, try to import it from the remote repo (this is only possible if we have ImageAdmin and ImageImportAdmin permissions)
**/
func (env *joyentEnviron) ensureImagePresent(imageUuid string) (*string, error) {
//func (env *joyentEnviron) ensureImagePresent(imageUuid string) (*cloudapi.Image, error) {
    availableImages, err := env.compute.cloudapi.ListAttachedImages()
	if err != nil {
		return nil, errors.Annotate(err, "unable to get available images from Danube Cloud")
	}

    for _, img := range availableImages {
        if img.Uuid == imageUuid {
            // we've found the image
            return &img.Name, nil
        }
    }

    logger.Debugf("The image \"%s\" was not found locally, trying to import from repo \"%s\"", imageUuid, remoteDanubeRepoName)
    errMsgGeneric := "Failed to retrieve requested image"

    remoteImages, err := env.compute.cloudapi.ListRemoteImages(remoteDanubeRepoName)
    if err != nil {
       if dcerrors.IsNotAuthorized(err) {
           // we do not have ImageImportAdmin rights
           // nothing else can be done
           return nil, errors.New("Requested image is not present on Danube Cloud installation and I don't have ImageImportAdmin rights to download it")
       } else {
           // generic error
           return nil, errors.Annotate(err, errMsgGeneric)
       }
    }

    if !isInList(imageUuid, remoteImages) {
        // try to update repo
        // but first chech last update time
        imgRepo, err := env.compute.cloudapi.GetImgRepo(remoteDanubeRepoName)
        if err != nil {
            return nil, errors.Annotate(err, errMsgGeneric)
        }

        if imgRepo.LastUpdate.Add(maxRepoRefreshAge).Before(time.Now()) {
            // repo needs update
            if env.compute.cloudapi.RefreshImgRepo(remoteDanubeRepoName) != nil {
                return nil, errors.Annotate(err, "Requested image is not present on Danube Cloud installation and remote repository refresh has failed during image import. Please try again later.")
            }
        }

        // try again
        remoteImages, err = env.compute.cloudapi.ListRemoteImages(remoteDanubeRepoName)
        if err != nil {
            return nil, errors.Annotate(err, errMsgGeneric)
        }
    }

    if !isInList(imageUuid, remoteImages) {
        return nil, errors.New("Requested image is not present on Danube Cloud installation and I also don't see it on a remote server. This might be a bug. Consider filling the issue at " + dcGithubRepo + "about inconsistent simplestreams image index")
    }

    // the remote image is found, let's download it
    // get the remote image name
    imgInfo, err := env.compute.cloudapi.GetRemoteImageInfo(imageUuid, remoteDanubeRepoName)
    if err != nil {
        return nil, errors.Annotate(err, errMsgGeneric)
    }
    // check if the image name is unused and alter it if neccessary
    newName := fmt.Sprintf("%s-%s", imgInfo.Name, imgInfo.Version)
    for i := 0; i < 100; i++ { // we don't expect 100 name collisions on the same version
        found := false
        if i > 0 {
			oldName := newName
            // format: imgname-1
            newName = fmt.Sprintf("%s-%s-%d", imgInfo.Name, imgInfo.Version, i)
			logger.Debugf("The image name \"%s\" is already taken. Trying \"%s\"", oldName, newName)
        }
        for _, img := range availableImages {
            if img.Name == imgInfo.Name {
                found = true
                break
            }
        }
        if !found {
            break
        }
		found = false
    }

    if env.compute.cloudapi.ImportImage(imageUuid, newName, remoteDanubeRepoName) != nil {
       if dcerrors.IsNotAuthorized(err) {
           // we do not have ImageImportAdmin rights
           // nothing else can be done
           return nil, errors.New("Requested image is not present on Danube Cloud installation and I don't have ImageAdmin and ImageImportAdmin rights to download it")
       } else if dcerrors.IsAlreadyExists(err) {
            // This is the most tricky state. The image is already imported but it is not attached to this datacenter.
            // The correction requires superadmin rights - to attach the image.
            // So we'll error now and let the user know what's going on.
            return nil, errors.New("The image is already present in Danube Cloud but it is not attached to our virtual datacenter. Please contact your Danube admin to attach the image \"" + imageUuid + "\" to your datacenter")
        }
        // generic error
        return nil, errors.New("Failed to import requested image from remote repository \"" + remoteDanubeRepoName + "\"")
    }

    logger.Debugf("The image \"%s\" was found and imported successfully", imageUuid)
    imgInfo, err = env.compute.cloudapi.GetAttachedImage(newName)
    if err != nil {
        return nil, errors.Annotate(err, errMsgGeneric)
    }

    return &imgInfo.Name, nil
}

