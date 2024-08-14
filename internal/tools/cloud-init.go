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
func (r *Remote) WaitForCloudInit(timeout string) {
	log.Println("[DEBUG] Waiting for cloud-init to finish timeout:", timeout)
	command := "[ -f /run/cloud-init/result.json ] && echo -n \"OK\""

	// Parse the timeout string into a time.Duration
	duration, err := time.ParseDuration(timeout)
	if err != nil {
		log.Fatalf("Invalid timeout value: %v", err)
	}

	timer := time.After(duration)

	for {
		select {
		case <-timer:
			log.Fatalln("Exiting.. Timeout reached while waiting for cloud-init to finish on IP " + r.IPAddress + " on port " + strconv.Itoa(r.SSHPort))
			return
		default:
			isOK, err := r.RemoteRun(&RemoteRunConfig{
				Command: command,
			})
			if err != nil {
				log.Println("[DEBUG] RemoteRun:" + err.Error())
			}
			if err == nil && isOK == "OK" {
				return
			}
			time.Sleep(3 * time.Second)
		}
	}
}
