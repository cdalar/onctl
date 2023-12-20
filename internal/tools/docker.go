package tools

import (
	"fmt"
	"log"
)

func RunRemoteBashScript(username, ip, privateKey, bashScript string) (string, error) {
	fmt.Print("Running Remote Bash Script...")
	log.Println("[DEBUG] scriptFile: " + bashScript)
	err := SSHCopyFile(username, ip, privateKey, bashScript, "./init.sh")
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

	// log.Println("[DEBUG] init.sh output: " + runInitOutput)
	// fmt.Println(runInitOutput)
	fmt.Println("DONE")
	return runInitOutput, err

}

// func RunDockerCompose(username, ip, privateKey, composeFile string) {

// 	// cmdCompose := exec.Command("DOCKER_HOST=ssh://ubuntu@$(cat ip.txt)", "docker", "compose", "up", "-d", "--build")
// 	// log.Println(*instance.PublicIpAddress)
// 	os.Setenv("DOCKER_HOST", "ssh://"+username+"@"+ip)
// 	cmdCompose := exec.Command("docker", "compose", "up", "-d", "--build")
// 	err := cmdCompose.Run()
// 	if err != nil {
// 		log.Println("Run Compose")
// 		log.Fatal(err)
// 	}
// 	out, err := cmdCompose.Output()
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	fmt.Println(string(out))
// 	log.Println("Service configured on:", "http://"+ip+"/")
// }
