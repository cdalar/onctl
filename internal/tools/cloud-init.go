package tools

import (
	"log"
	"strconv"
	"time"
)

func WaitForCloudInit(username, ip string, privateKey string) {
	var tries int
	for {

		isOK, err := RemoteRun(username, ip, privateKey, "[ -f /run/cloud-init/result.json ] && echo -n \"OK\"")
		if err != nil {
			log.Println("[DEBUG] RemoteRun:" + err.Error())
		}
		if err == nil {
			log.Println("Server started.")
			if isOK == "OK" {
				break
			}
		}
		time.Sleep(3 * time.Second)
		tries++
		log.Println("[DEBUG] :" + strconv.Itoa(tries))
		if tries > 15 {
			log.Fatalln("Exiting.. Could not connect to IP " + ip)
		}
	}
}
