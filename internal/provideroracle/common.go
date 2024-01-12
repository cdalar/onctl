package provideroracle

import (
	"log"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/example/helpers"
)

func GetComputeClient() core.ComputeClient {
	// config := common.DefaultConfigProvider()
	// config := common.CustomProfileSessionTokenConfigProvider("/Users/cd/.oci/config", "DEFAULT")
	// config := common.CustomProfileConfigProvider("/Users/cd/.oci/config", "DEFAULT")
	phrase := "1234"
	config := common.NewRawConfigurationProvider(
		"ocid1.tenancy.oc1..aaaaaaaalu2nragvcuhjv74ljruxa7y2phau2qougo5idzkupnqvy2qmt3ma",
		"ocid1.user.oc1..aaaaaaaaea2aatszd4llej7qpd5szznvxnhylusw7h743yrcvl62occ2yb6a",
		"eu-amsterdam-1",
		"5a:ec:76:78:fd:05:a2:15:68:a6:d9:12:47:62:47:89",
		"/Users/cd/.oci/sessions/cdalar/oci_api_key.pem",
		&phrase,
	)
	log.Println(config)
	client, err := core.NewComputeClientWithConfigurationProvider(config)
	if err != nil {
		log.Fatalln(err)
	}

	// helpers.FatalIfError(err)
	return client
}

func GetBaseClient() common.BaseClient {
	client, err := common.NewClientWithConfig(common.DefaultConfigProvider())
	helpers.FatalIfError(err)
	return client
}
