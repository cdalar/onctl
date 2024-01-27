package tools

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cdalar/onctl/internal/rand"
	"golang.org/x/crypto/ssh"
)

const (
	REMOTEDIR = ".onctl"
)

type RemoteRunBashScriptConfig struct {
	Username   string
	IPAddress  string
	SSHPort    int
	PrivateKey string
	Script     string
	Vars       []string
	IsApply    bool
}

type RemoteRunConfig struct {
	Username   string
	IPAddress  string
	SSHPort    int
	PrivateKey string
	Command    string
}

// e.g. output, err := remoteRun("root", "MY_IP", "PRIVATE_KEY", "ls")
func RemoteRun(remoteRunConfig *RemoteRunConfig) (string, error) {
	key, err := ssh.ParsePrivateKey([]byte(remoteRunConfig.PrivateKey))
	if err != nil {
		return "", err
	}
	// Authentication
	config := &ssh.ClientConfig{
		User:            remoteRunConfig.Username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Second * 7,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(key),
		},
	}
	// Connect
	client, err := ssh.Dial("tcp", net.JoinHostPort(remoteRunConfig.IPAddress, fmt.Sprint(remoteRunConfig.SSHPort)), config)
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
	stdOutReader, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}

	err = session.Run(remoteRunConfig.Command)
	buf := make([]byte, 1024)
	var returnString string
	for {
		n, err := stdOutReader.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println(err)
			continue
		}
		if n > 0 {
			log.Println("[DEBUG] " + string(buf[:n]))
			// fmt.Print(string(buf[:n]))
			returnString += string(buf[:n])
		}
	}
	return returnString, err
}

func RemoteRunBashScript(config *RemoteRunBashScriptConfig) (string, error) {
	var (
		command string
		dstPath string
	)
	randomString := rand.String(5)

	for _, value := range config.Vars {
		envs := strings.Split(value, "=")
		vars_command := envs[0] + "=" + envs[1]
		command += vars_command + " "
	}
	log.Println("[DEBUG] command: " + command)
	// Create REMOTEDIR folder
	if config.IsApply {
		command = "mkdir -p " + REMOTEDIR + "/apply-" + randomString
	} else {
		command = "mkdir -p " + REMOTEDIR
	}

	runInitOutput, err := RemoteRun(&RemoteRunConfig{
		Username:   config.Username,
		IPAddress:  config.IPAddress,
		SSHPort:    config.SSHPort,
		PrivateKey: config.PrivateKey,
		Command:    command,
	})
	if err != nil {
		fmt.Println(runInitOutput)
		log.Fatalln(err)
	}

	fileBaseName := filepath.Base(config.Script)
	// Extract tar.gz
	if slices.Contains([]string{".tgz", ".gz"}, filepath.Ext(config.Script)) {
		// Copy bash script or tar.gz to .onctl-init folder
		err = SSHCopyFile(config.Username, config.IPAddress, config.SSHPort, config.PrivateKey, config.Script, REMOTEDIR+"/"+fileBaseName)
		if err != nil {
			log.Fatalln(err)
		}
		runInitOutput, err = RemoteRun(&RemoteRunConfig{
			Username:   config.Username,
			IPAddress:  config.IPAddress,
			SSHPort:    config.SSHPort,
			PrivateKey: config.PrivateKey,
			Command:    "cd " + REMOTEDIR + " && tar -xzf " + fileBaseName,
		})
		if err != nil {
			fmt.Println(runInitOutput)
			log.Fatalln(err)
		}
	} else { // not tar.gz
		if config.IsApply {
			dstPath = REMOTEDIR + "/apply-" + randomString + "/" + fileBaseName
		} else {
			dstPath = REMOTEDIR + "/" + fileBaseName
		}
		// Checking file in filesystem
		_, err := os.Stat(filepath.Dir(config.Script) + "/.env")
		if err == nil { // file found in filesystem
			log.Println("[DEBUG]", ".env file found in filesystem, trying to copy to remote")
			err = SSHCopyFile(config.Username, config.IPAddress, config.SSHPort, config.PrivateKey, filepath.Dir(config.Script)+"/.env", filepath.Dir(dstPath)+"/.env")
			if err != nil {
				log.Fatalln(err)
			}
		}

		log.Println("[DEBUG] copying " + config.Script + " to remote...")
		err = SSHCopyFile(config.Username, config.IPAddress, config.SSHPort, config.PrivateKey, config.Script, dstPath)
		if err != nil {
			log.Fatalln(err)
		}
	}

	log.Println("[DEBUG] running " + fileBaseName + "...")
	if config.IsApply {
		command = "cd " + REMOTEDIR + "/apply-" + randomString + " && chmod +x " + fileBaseName + " && if [[ -f .env ]]; then set -o allexport; source .env; set +o allexport; fi && ./" + fileBaseName + "> output-" + fileBaseName + ".log 2>&1"
	} else {
		command = "cd " + REMOTEDIR + " && chmod +x " + fileBaseName + " && if [[ -f .env ]]; then set -o allexport; source .env; set +o allexport; fi && ./" + fileBaseName + "> output-" + fileBaseName + ".log 2>&1"
	}
	log.Println("[DEBUG] command: " + command)
	runInitOutput, err = RemoteRun(&RemoteRunConfig{
		Username:   config.Username,
		IPAddress:  config.IPAddress,
		SSHPort:    config.SSHPort,
		PrivateKey: config.PrivateKey,
		Command:    command,
	})
	if err != nil {
		log.Println("Error on remoteRun")
		fmt.Println(runInitOutput)
		log.Fatalln(err)
	}

	log.Println("[DEBUG] init.sh output: " + runInitOutput)
	return runInitOutput, err

}
