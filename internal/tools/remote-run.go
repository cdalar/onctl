package tools

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/cdalar/onctl/internal/rand"
	"golang.org/x/crypto/ssh"
)

type RunRemoteBashScriptConfig struct {
	Username   string
	IPAddress  string
	SSHPort    string
	PrivateKey string
	Script     string
	IsApply    bool
}

// e.g. output, err := remoteRun("root", "MY_IP", "PRIVATE_KEY", "ls")
func RemoteRun(user string, addr string, sshPort string, privateKey string, cmd string) (string, error) {
	key, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return "", err
	}
	// Authentication
	config := &ssh.ClientConfig{
		User: user,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			// Always accept key.
			return nil
		},
		Timeout: time.Second * 7,
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
	stdOutReader, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}

	err = session.Run(cmd)
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

func RunRemoteBashScript(config *RunRemoteBashScriptConfig) (string, error) {
	var (
		command string
		dstPath string
	)
	randomString := rand.String(5)

	// Create .onctl-init folder
	if config.IsApply {
		command = "mkdir -p .onctl-init/apply-" + randomString
	} else {
		command = "mkdir -p .onctl-init"
	}

	runInitOutput, err := RemoteRun(config.Username, config.IPAddress, config.SSHPort, config.PrivateKey, command)
	if err != nil {
		fmt.Println(runInitOutput)
		log.Fatalln(err)
	}

	fileBaseName := filepath.Base(config.Script)
	// Extract tar.gz
	if slices.Contains([]string{".tgz", ".gz"}, filepath.Ext(config.Script)) {
		// Copy bash script or tar.gz to .onctl-init folder
		err = SSHCopyFile(config.Username, config.IPAddress, config.SSHPort, config.PrivateKey, config.Script, ".onctl-init/"+fileBaseName)
		if err != nil {
			log.Fatalln(err)
		}
		runInitOutput, err = RemoteRun(config.Username, config.IPAddress, config.SSHPort, config.PrivateKey, "cd .onctl-init && tar -xzf "+fileBaseName)
		if err != nil {
			fmt.Println(runInitOutput)
			log.Fatalln(err)
		}
	} else {
		if config.IsApply {
			dstPath = ".onctl-init/apply-" + randomString + "/" + fileBaseName
		} else {
			dstPath = ".onctl-init/" + fileBaseName
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

		err = SSHCopyFile(config.Username, config.IPAddress, config.SSHPort, config.PrivateKey, config.Script, dstPath)
		if err != nil {
			log.Fatalln(err)
		}
	}

	log.Println("[DEBUG] running " + fileBaseName + "...")
	if config.IsApply {
		command = "cd .onctl-init/apply-" + randomString + " && chmod +x " + fileBaseName + " && if [[ -f .env ]]; then set -o allexport; source .env; set +o allexport; fi && sudo -E ./" + fileBaseName + "> output-" + fileBaseName + ".log 2>&1"
	} else {
		command = "cd .onctl-init && chmod +x " + fileBaseName + " && if [[ -f .env ]]; then set -o allexport; source .env; set +o allexport; fi && sudo -E ./" + fileBaseName + "> output-" + fileBaseName + ".log 2>&1"
	}
	log.Println("[DEBUG] command: " + command)
	runInitOutput, err = RemoteRun(config.Username, config.IPAddress, config.SSHPort, config.PrivateKey, command)
	if err != nil {
		log.Println("Error on remoteRun")
		fmt.Println(runInitOutput)
		log.Fatalln(err)
	}

	log.Println("[DEBUG] init.sh output: " + runInitOutput)
	fmt.Println(runInitOutput)
	return runInitOutput, err

}
