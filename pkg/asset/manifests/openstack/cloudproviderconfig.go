package openstack

import (
	"os"
	"strconv"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/utils/openstack/clientconfig"
	networkutils "github.com/gophercloud/utils/openstack/networking/v2/networks"

	"github.com/openshift/installer/pkg/asset/installconfig/openstack"
	"github.com/openshift/installer/pkg/types"
)

// Error represents a failure while generating OpenStack provider
// configuration.
type Error struct {
	err error
	msg string
}

func (e Error) Error() string { return e.msg + ": " + e.err.Error() }
func (e Error) Unwrap() error { return e.err }

// CloudProviderConfigSecret generates the cloud provider config for the OpenStack
// platform, that will be stored in the system secret.
func CloudProviderConfigSecret(cloud *clientconfig.Cloud) ([]byte, error) {
	domainID := cloud.AuthInfo.DomainID
	if domainID == "" {
		domainID = cloud.AuthInfo.UserDomainID
	}

	domainName := cloud.AuthInfo.DomainName
	if domainName == "" {
		domainName = cloud.AuthInfo.UserDomainName
	}

	// We have to generate this config manually without "go-ini" library, because its
	// output data is incompatible with "gcfg".
	// For instance, if there is a string with a # character, then "go-ini" wraps it in bacticks,
	// like `aaa#bbb`, but gcfg doesn't recognize it and  parses the data as `aaa, skipping
	// everything after the #.
	// For more information: https://bugzilla.redhat.com/show_bug.cgi?id=1771358
	var res strings.Builder
	res.WriteString("[Global]\n")
	if cloud.AuthInfo.AuthURL != "" {
		res.WriteString("auth-url = " + strconv.Quote(cloud.AuthInfo.AuthURL) + "\n")
	}
	if cloud.AuthInfo.Username != "" {
		res.WriteString("username = " + strconv.Quote(cloud.AuthInfo.Username) + "\n")
	}
	if cloud.AuthInfo.Password != "" {
		res.WriteString("password = " + strconv.Quote(cloud.AuthInfo.Password) + "\n")
	}
	if cloud.AuthInfo.ProjectID != "" {
		res.WriteString("tenant-id = " + strconv.Quote(cloud.AuthInfo.ProjectID) + "\n")
	}
	if cloud.AuthInfo.ProjectName != "" {
		res.WriteString("tenant-name = " + strconv.Quote(cloud.AuthInfo.ProjectName) + "\n")
	}
	if domainID != "" {
		res.WriteString("domain-id = " + strconv.Quote(domainID) + "\n")
	}
	if domainName != "" {
		res.WriteString("domain-name = " + strconv.Quote(domainName) + "\n")
	}
	if cloud.RegionName != "" {
		res.WriteString("region = " + strconv.Quote(cloud.RegionName) + "\n")
	}
	if cloud.CACertFile != "" {
		res.WriteString("ca-file = /etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem\n")
	}

	return []byte(res.String()), nil
}

func generateCloudProviderConfig(networkClient *gophercloud.ServiceClient, cloudConfig *clientconfig.Cloud, installConfig types.InstallConfig) (cloudProviderConfigData, cloudProviderConfigCABundleData string, err error) {
	cloudProviderConfigData = `[Global]
secret-name = openstack-credentials
secret-namespace = kube-system
`
	if regionName := cloudConfig.RegionName; regionName != "" {
		cloudProviderConfigData += "region = " + regionName + "\n"
	}

	if caCertFile := cloudConfig.CACertFile; caCertFile != "" {
		cloudProviderConfigData += "ca-file = /etc/kubernetes/static-pod-resources/configmaps/cloud-config/ca-bundle.pem\n"
		caFile, err := os.ReadFile(caCertFile)
		if err != nil {
			return "", "", Error{err, "failed to read clouds.yaml ca-cert from disk"}
		}
		cloudProviderConfigCABundleData = string(caFile)
	}

	if installConfig.OpenStack.ExternalNetwork != "" {
		networkName := installConfig.OpenStack.ExternalNetwork // Yes, we use a name in install-config.yaml :/
		networkID, err := networkutils.IDFromName(networkClient, networkName)
		if err != nil {
			return "", "", Error{err, "failed to fetch external network " + networkName}
		}
		// If set get the ID and configure CCM to use that network for LB FIPs.
		cloudProviderConfigData += "\n[LoadBalancer]\n"
		cloudProviderConfigData += "floating-network-id = " + networkID + "\n"
	}

	return cloudProviderConfigData, cloudProviderConfigCABundleData, nil
}

func getNetworkClient(session *openstack.Session) (*gophercloud.ServiceClient, error) {
	return clientconfig.NewServiceClient("network", session.ClientOpts)
}

// GenerateCloudProviderConfig adds the cloud provider config for the OpenStack
// platform in the provided configmap.
func GenerateCloudProviderConfig(installConfig types.InstallConfig) (cloudProviderConfigData, cloudProviderConfigCABundleData string, err error) {
	cloud, err := openstack.GetSession(installConfig.Platform.OpenStack.Cloud)
	if err != nil {
		return "", "", Error{err, "failed to get cloud config for openstack"}
	}

	networkClient, err := getNetworkClient(cloud)
	if err != nil {
		return "", "", Error{err, "failed to create a network client"}
	}

	return generateCloudProviderConfig(networkClient, cloud.CloudConfig, installConfig)
}
