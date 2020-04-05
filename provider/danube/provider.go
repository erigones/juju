// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package danube

import (
	"log"
	"net/url"

	"github.com/erigones/godanube/client"
	joyenterrors "github.com/erigones/godanube/errors"
	"github.com/erigones/godanube/cloudapi"
	"github.com/erigones/godanube/auth"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/simplestreams"
)

var logger = loggo.GetLogger("juju.provider.joyent")

// TODO(ericsnow) gologWriter can go away once loggo.Logger has a GoLogger() method.

type gologWriter struct {
	loggo.Logger
	level loggo.Level
}

func newGoLogger() *log.Logger {
	return log.New(&gologWriter{logger, loggo.DEBUG}, "", 0)
}

func (w *gologWriter) Write(p []byte) (n int, err error) {
	w.Logf(w.level, string(p))
	return len(p), nil
}

type joyentProvider struct {
	environProviderCredentials
}

var providerInstance = joyentProvider{}
var _ environs.EnvironProvider = providerInstance

var _ simplestreams.HasRegion = (*joyentEnviron)(nil)

/* J XXX DELME
// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p joyentProvider) CloudSchema() *jsonschema.Schema {
	return nil
}*/

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey, cloud.AuthTypesKey},
	Order:    []string{cloud.EndpointKey, cloud.AuthTypesKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: {
			Singular: "the Danube Cloud mgmt address or URL",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
		cloud.AuthTypesKey: {
			Singular: "API key",
			Plural:   "API keys",
			Type: []jsonschema.Type{jsonschema.ArrayType},
			Enum: []interface{}{[]string{string(cloud.AccessKeyAuthType)}},
		},
		/*
		cloud.RegionsKey: {
			Type:     []jsonschema.Type{jsonschema.StringType},
			Singular: "virtual datacenter name inside Danube Cloud",
			Plural:   "virtual datacenters",
			Default:  "main",
			/*
			AdditionalProperties: &jsonschema.Schema{
				Type:          []jsonschema.Type{jsonschema.StringType},
				MaxProperties: jsonschema.Int(0),
			},* /
		},*/
	},
}

// CloudSchema returns the schema used to validate input for add-cloud.
func (p joyentProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

func getImageSource(env environs.Environ) (simplestreams.DataSource, error) {
	/*
	_, ok := env.(*joyentEnviron)
	if !ok {
		return nil, errors.NotSupportedf("non-danube model")
	}
	*/
	dataSourceConfig := simplestreams.Config{
		Description:          "danube cloud images",
		//BaseURL:              fmt.Sprintf(CloudsigmaCloudImagesURLTemplate, e.cloud.Region),
		//BaseURL:              "http://10.100.10.162:5555/",
		BaseURL:              "https://images.danube.cloud/",
		//XXX 2DO
		//HostnameVerification: utils.VerifySSLHostnames,
		Priority:             simplestreams.SPECIFIC_CLOUD_DATA,
	}
	if err := dataSourceConfig.Validate(); err != nil {
		return nil, errors.Annotate(err, "simplestreams config validation failed")
	}
	return simplestreams.NewDataSource(dataSourceConfig), nil
}

/*
// Ping tests the connection to the cloud, to verify the endpoint is valid.
//J TODO DELME
func (p joyentProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}*/

// if the endpoint is IP address, make it into an URL
func FormatDanubeUrl(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	switch u.Scheme {
	case "http", "https":
		// good!
		return endpoint
	case "":
		endpoint = "https://" + endpoint + "/api/"
		u, err = url.Parse(endpoint)
		if err != nil {
			return ""
		}
		return endpoint
	}
	return ""
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p joyentProvider) Ping(callCtx context.ProviderCallContext, endpoint string) error {
	// try to be smart and not punish people for adding or forgetting http
	endpoint = FormatDanubeUrl(endpoint)
	if endpoint == "" {
		return errors.New("Invalid endpoint format, please give a full URL or IP/hostname.")
	}

	fakeAuth, _ := auth.NewAuth("", "", "juju-fake-api-key")
	fakeCreds := &auth.Credentials{
		UserAuthentication:		fakeAuth,
		ApiEndpoint:			auth.Endpoint{URL: endpoint},
		VirtDatacenter:			"",	// default
	}

	httpClient := client.NewClient(fakeCreds, cloudapi.DefaultAPIVersion, newGoLogger())
	apiClient := cloudapi.New(httpClient)

	if logger.EffectiveLogLevel() == loggo.DEBUG {
		apiClient.SetTrace(true)
	}

	_, err := apiClient.GetRunningTasks()
	if err != nil && joyenterrors.IsNotAuthorized(err) {
		// the server correctly responded to incorrect credentials
		return nil
	} else if err == nil {
		// correct api key? Very improbable. But the api call succeeded so we continue.
		return nil
	} else {
		logger.Errorf("Error connecting to Danube Cloud: %v", err)
		return errors.Errorf("Unable to contact Danube Cloud at %s", endpoint)
	}
}

// PrepareConfig is part of the EnvironProvider interface.
func (p joyentProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	return args.Config, nil
}

const unauthorisedMessage = `
Please ensure the Danube Cloud API key you have provided is correct.
You can find API key in GUI under user Profile -> API keys -> API key.`
/*
const unauthorisedMessage = `
Please ensure the SSH access key you have specified is correct.
You can create or import an SSH key via the "Account Summary"
page in the Joyent console.`
*/
// verifyCredentials issues a cheap, non-modifying request to Danube Cloud to
// verify the configured credentials. If verification fails, a user-friendly
// error will be returned, and the original error will be logged at debug
// level.
// GetRunningTasks() call is accessible to every user therefore IsNotAuthorized
// means wrong API key.
var verifyCredentials = func(e *joyentEnviron) error {
//J
	creds, err := credentials(e.cloud)
	if err != nil {
		return err
	}

	httpClient := client.NewClient(creds, cloudapi.DefaultAPIVersion, newGoLogger())
	apiClient := cloudapi.New(httpClient)

	if logger.EffectiveLogLevel() == loggo.DEBUG {
		apiClient.SetTrace(true)
	}

	_, err = apiClient.GetRunningTasks()
	if err != nil {
		logger.Debugf("Danube Cloud request failed: %v", err)
		if joyenterrors.IsNotAuthorized(err) {
			return errors.New("authentication failed.\n" + unauthorisedMessage)
		}
		return err
	}
	return nil
}

func credentials(cloud environs.CloudSpec) (*auth.Credentials, error) {
	credAttrs := cloud.Credential.Attributes()
	ApiKey := credAttrs[credAttrApiKey]
	VirtDc := credAttrs[credAttrVirtDc]
	//sdcUser := credAttrs[credAttrSDCUser]
	//sdcKeyID := credAttrs[credAttrSDCKeyID]
	//privateKey := credAttrs[credAttrPrivateKey]
	//algorithm := credAttrs[credAttrAlgorithm]
	//if algorithm == "" {
	//	algorithm = algorithmDefault
	//}

	//authentication, err := auth.NewAuth(sdcUser, privateKey, algorithm)
	authentication, err := auth.NewAuth("", "", ApiKey)
	if err != nil {
		return nil, errors.Errorf("cannot create credentials: %v", err)
	}
	return &auth.Credentials{
		//J
		UserAuthentication: authentication,
		// the output of FormatDanubeUrl() was already verified in Ping()
		ApiEndpoint:        auth.Endpoint{URL: FormatDanubeUrl(cloud.Endpoint)},
		VirtDatacenter:     VirtDc,
	}, nil
}

// Version is part of the EnvironProvider interface.
func (joyentProvider) Version() int {
	return 0
}

func (joyentProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(args.Cloud, args.Config)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func (joyentProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	newEcfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, errors.Errorf("invalid Joyent provider config: %v", err)
	}
	return cfg.Apply(newEcfg.attrs)
}

func GetProviderInstance() environs.EnvironProvider {
	return providerInstance
}

// MetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p joyentProvider) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		return nil, errors.Errorf("region must be specified")
	}
	return &simplestreams.MetadataLookupParams{
		Region: region,
	}, nil
}

func (p joyentProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.AccessKeyAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
