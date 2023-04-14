package tools

import (
	"fmt"
	"log"
	"os"
	"os/exec"
)

func PrepareDocker(username, ip, privateKey, initFile string) {
	fmt.Print("Preparing docker...")
	log.Println("[DEBUG] initFile: " + initFile)
	err := SSHCopyFile(username, ip, privateKey, initFile, "./init.sh")
	if err != nil {
		log.Println("Error on copy Init")
		log.Fatalln(err)
	}

	log.Println("[DEBUG] running init.sh...")
	runInitOutput, err := RemoteRun(username, ip, privateKey, "chmod +x init.sh && sudo ./init.sh")
	if err != nil {
		log.Println("Error on init.sh")
		fmt.Println(runInitOutput)
		log.Fatalln(err)
	}

	log.Println("[DEBUG] init.sh output: " + runInitOutput)
	if username != "root" {
		_, err := RemoteRun(username, ip, privateKey, "sudo usermod -aG docker ubuntu")
		if err != nil {
			log.Println("Error on usermod")
			log.Fatalln(err)
		}
	}
	fmt.Println("DONE")
}

func RunDockerCompose(username, ip, privateKey, composeFile string) {

	// cmdCompose := exec.Command("DOCKER_HOST=ssh://ubuntu@$(cat ip.txt)", "docker", "compose", "up", "-d", "--build")
	// log.Println(*instance.PublicIpAddress)
	os.Setenv("DOCKER_HOST", "ssh://"+username+"@"+ip)
	cmdCompose := exec.Command("docker", "compose", "up", "-d", "--build")
	err := cmdCompose.Run()
	if err != nil {
		log.Println("Run Compose")
		log.Fatal(err)
	}
	out, err := cmdCompose.Output()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(out))
	log.Println("Service configured on:", "http://"+ip+"/")
}
