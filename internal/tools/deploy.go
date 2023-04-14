package tools

import (
	"encoding/json"
	"log"
	"os"
)

type DeployOutput struct {
	Username   string `json:"username"`
	PublicURL  string `json:"public_url"`
	PublicIP   string `json:"public_ip"`
	DockerHost string `json:"docker_host"`
}

func CreateDeployOutputFile(deployOutput *DeployOutput) {
	json, err := json.MarshalIndent(deployOutput, "", "  ")
	if err != nil {
		log.Println(err)
	}
	file, err := os.Create("onctl-deploy.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	_, err = file.Write(json)
	if err != nil {
		log.Fatal(err)
	}

	err = file.Sync()
	if err != nil {
		log.Fatal(err)
	}
}

func StringSliceToPointerSlice(strSlice []string) []*string {
	ptrSlice := make([]*string, len(strSlice))
	for i, str := range strSlice {
		ptrSlice[i] = &str
	}
	return ptrSlice
}
