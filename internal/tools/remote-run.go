package tools

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// e.g. output, err := remoteRun("root", "MY_IP", "PRIVATE_KEY", "ls")
func RemoteRun(user string, addr string, sshPort string, privateKey string, cmd string) (string, error) {
	key, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return "", err
	}
	// Authentication
	config := &ssh.ClientConfig{
		User:            user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Minute * 5,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(key),
		},
	}
	// Connect
	client, err := ssh.Dial("tcp", net.JoinHostPort(addr, sshPort), config)
	if err != nil {
		return "", err
	}
	defer client.Close()
	// Create a session. It is one session per command.
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var b bytes.Buffer  // import "bytes"
	session.Stdout = &b // get output
	err = session.Run(cmd)
	return b.String(), err
}

func RunRemoteBashScript(username, ip, sshPort, privateKey, bashScript string) (string, error) {
	fmt.Print("Running Remote Bash Script...")
	log.Println("[DEBUG] scriptFile: " + bashScript)
	err := SSHCopyFile(username, ip, privateKey, bashScript, "./init.sh")
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

	// log.Println("[DEBUG] init.sh output: " + runInitOutput)
	// fmt.Println(runInitOutput)
	fmt.Println("DONE")
	return runInitOutput, err

}
