// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package danube

import "github.com/juju/juju/environs"

const (
	providerType = "danube"
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)
	environs.RegisterImageDataSourceFunc("danube cloud image source", getImageSource)
    //DELME XXX
	//environs.RegisterUserImageDataSourceFunc("danube cloud image source", getImageSource)
}
