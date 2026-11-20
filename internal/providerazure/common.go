package providerazure

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/spf13/viper"
)

func GetResourceGraphClient() (*armresourcegraph.Client, error) {
	cred, err := connectionAzure()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Azure: %w", err)
	}
	client, err := armresourcegraph.NewClient(cred, nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func GetVmClient() (*armcompute.VirtualMachinesClient, error) {
	cred, err := connectionAzure()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Azure: %w", err)
	}
	client, err := armcompute.NewVirtualMachinesClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func GetNicClient() (*armnetwork.InterfacesClient, error) {
	cred, err := connectionAzure()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Azure: %w", err)
	}
	client, err := armnetwork.NewInterfacesClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func GetSSHKeyClient() (*armcompute.SSHPublicKeysClient, error) {
	cred, err := connectionAzure()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Azure: %w", err)
	}
	client, err := armcompute.NewSSHPublicKeysClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func GetIPClient() (*armnetwork.PublicIPAddressesClient, error) {
	cred, err := connectionAzure()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Azure: %w", err)
	}
	client, err := armnetwork.NewPublicIPAddressesClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func GetVnetClient() (*armnetwork.VirtualNetworksClient, error) {
	cred, err := connectionAzure()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Azure: %w", err)
	}
	client, err := armnetwork.NewVirtualNetworksClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func GetNSGClient() (*armnetwork.SecurityGroupsClient, error) {
	cred, err := connectionAzure()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to Azure: %w", err)
	}
	client, err := armnetwork.NewSecurityGroupsClient(viper.GetString("azure.subscriptionId"), cred, nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func connectionAzure() (azcore.TokenCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	return cred, nil
}

func runAzCLI(args ...string) string {
	if _, err := exec.LookPath("az"); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "az", args...).Output()
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(out))
	if value == "" || value == "null" {
		return ""
	}
	return value
}

// AzureCLISubscriptionID best-effort resolves the az CLI's active
// subscription (the same one `az` commands run against), so onctl can
// default azure.subscriptionId to it when the value in onctl.yaml is still
// the placeholder or unset. Returns "" if az isn't installed/logged in or errors.
func AzureCLISubscriptionID() string {
	return runAzCLI("account", "show", "--query", "id", "-o", "tsv")
}

// AzureCLIDefaultResourceGroup best-effort resolves the az CLI's configured
// default resource group (`az configure --defaults group=<name>`).
// Most users won't have this set (opt-in), so empty is normal.
func AzureCLIDefaultResourceGroup() string {
	return runAzCLI("config", "get", "defaults.group", "--query", "value", "-o", "tsv")
}
