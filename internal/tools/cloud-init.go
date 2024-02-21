package tools

import (
	"encoding/base64"
	"log"
	"os"
	"strconv"
	"time"
)

func FileToBase64(filepath string) string {
	if filepath == "" {
		return ""
	}
	// Check if file exists
	if _, err := os.Stat(filepath); err != nil {
		log.Println("FileToBase64:" + err.Error())
		log.Println("Setting empty cloud-init file")
		return ""
	}

	// Read the file
	data, err := os.ReadFile(filepath)
	if err != nil {
		log.Fatal(err)
	}
	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(data)
	return encoded
}

// WaitForCloudInit waits for cloud-init to finish
func (r *Remote) WaitForCloudInit() {
	var tries int

	command := "[ -f /run/cloud-init/result.json ] && echo -n \"OK\""
	for {

		isOK, err := r.RemoteRun(&RemoteRunConfig{
			Command: command,
		})
		if err != nil {
			log.Println("[DEBUG] RemoteRun:" + err.Error())
		}
		if err == nil {
			if isOK == "OK" {
				break
			}
		}
		time.Sleep(3 * time.Second)
		tries++
		log.Println("[DEBUG] :" + strconv.Itoa(tries))
		if tries > 15 {
			log.Fatalln("Exiting.. Could not connect to IP " + r.IPAddress + " on port " + strconv.Itoa(r.SSHPort))
		}
	}
}
