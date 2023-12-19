package provideroracle

import (
	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/core"
	"github.com/oracle/oci-go-sdk/example/helpers"
)

func GetComputeClient() core.ComputeClient {
	client, err := core.NewComputeClientWithConfigurationProvider(common.DefaultConfigProvider())
	helpers.FatalIfError(err)
	return client
}

func GetBaseClient() common.BaseClient {
	client, err := common.NewClientWithConfig(common.DefaultConfigProvider())
	helpers.FatalIfError(err)
	return client
}
