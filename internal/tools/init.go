package tools

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/cdalar/onctl/internal/files"
)

func GetCustomData() string {
	fileEmbeded, err := files.EmbededFiles.ReadFile("custom-data.sh")
	if err != nil {
		log.Fatal(err)
	}
	encodedData := base64.StdEncoding.EncodeToString(fileEmbeded)
	if err != nil {
		log.Fatal(err)
	}
	return string(encodedData)
}

func RunLocalInit(username, ip, privateKey, initFile string) {
	fmt.Print("Running LocalInit Script...")
	log.Println("[DEBUG] localInitFile: " + initFile)

	cmdChmod := exec.Command("chmod", "+x", initFile)
	err := cmdChmod.Run()
	if err != nil {
		log.Println("Error on chmod +x " + initFile)
		log.Fatalln(err)
	}
	cmdLocalInit := exec.Command("./init_local.sh")
	cmdLocalInit.Env = append(os.Environ(), "IP="+ip, "USERNAME="+username, "PRIVATE_KEY="+privateKey)
	cmdLocalInit.Stdout = os.Stdout
	cmdLocalInit.Stderr = os.Stderr
	err = cmdLocalInit.Run()
	if err != nil {
		log.Println("Error on init_local.sh")
		log.Fatalln(err)
	}
	fmt.Println("DONE")
}

func RunInit(username, ip, sshPort, privateKey, initFile string) {
	fmt.Print("Running Init Script...")
	log.Println("[DEBUG] initFile: " + initFile)
	err := SSHCopyFile(username, ip, sshPort, privateKey, initFile, "./init.sh")
	if err != nil {
		log.Println("Error on copy Init")
		log.Fatalln(err)
	}

	log.Println("[DEBUG] running init.sh...")
	runInitOutput, err := RemoteRun(username, ip, sshPort, privateKey, "chmod +x init.sh && sudo ./init.sh")
	if err != nil {
		log.Println("Error on init.sh")
		fmt.Println(runInitOutput)
		log.Fatalln(err)
	}

	log.Println("[DEBUG] init.sh output: " + runInitOutput)
	if username != "root" {
		_, err := RemoteRun(username, ip, sshPort, privateKey, "sudo usermod -aG docker ubuntu")
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
