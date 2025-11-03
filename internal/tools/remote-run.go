package tools

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cdalar/onctl/internal/files"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const (
	ONCTLDIR = ".onctl"
)

type Remote struct {
	Username   string
	IPAddress  string
	SSHPort    int
	PrivateKey string
	Passphrase string
	Spinner    *spinner.Spinner
	Client     *ssh.Client
}

type RemoteRunConfig struct {
	Command string
	Vars    []string
}

type CopyAndRunRemoteFileConfig struct {
	File string
	Vars []string
}

func (r *Remote) ReadPassphrase() (string, error) {
	// fmt.Println("Error: Passphrase is missing for the private key")
	fmt.Print("Enter passphrase for private key:")

	// Turn off input echoing
	bytePassphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Println() // Print a newline after the password input
	r.Passphrase = string(bytePassphrase)
	return string(bytePassphrase), nil
}

func (r *Remote) NewSSHConnection() error {
	var (
		key ssh.Signer
		err error
	)
	if r.Client != nil {
		return nil
	}
	if r.Passphrase != "" {
		key, err = ssh.ParsePrivateKeyWithPassphrase([]byte(r.PrivateKey), []byte(r.Passphrase))
	} else {
		key, err = ssh.ParsePrivateKey([]byte(r.PrivateKey))
	}
	if err != nil {
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			// fmt.Println("Error: Passphrase is missing for the private key")
			if r.Spinner != nil {
				r.Spinner.Stop()
			}
			passphrase, err := r.ReadPassphrase()
			if r.Spinner != nil {
				r.Spinner.Restart()
			}
			if err != nil {
				log.Fatalln("Error reading passphrase: ", err)
			}
			key, err = ssh.ParsePrivateKeyWithPassphrase([]byte(r.PrivateKey), []byte(passphrase))
			if err != nil {
				log.Fatalln("Error parsing private key: ", err)
			}
		} else {
			log.Fatalln("Error parsing private key: ", err)
			return err
		}
	}
	// Authentication
	config := &ssh.ClientConfig{
		User:            r.Username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Second * 7,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(key),
		},
	}
	// Connect
	r.Client, err = ssh.Dial("tcp", net.JoinHostPort(r.IPAddress, fmt.Sprint(r.SSHPort)), config)
	if err != nil {
		return err
	}
	return nil
}

// exists returns whether the given file or directory exists
func exists(path string) (bool, error) {
	fmt.Println("Checking if ", path, " exists")
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func ParseDotEnvFile(dotEnvFile string) ([]string, error) {
	var vars []string
	file, err := os.Open(dotEnvFile)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close file: %v", err)
		}
	}()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.Trim(line, " ")
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		vars = append(vars, line)
	}
	return vars, nil
}

func variablesToEnvVars(vars []string) string {
	if len(vars) == 0 {
		return ""
	}

	var command string
	for _, value := range vars {
		envs := strings.SplitN(value, "=", 2)
		if len(envs) == 1 {
			envs = append(envs, os.Getenv(envs[0]))
		}
		vars_command := envs[0] + "=" + strconv.Quote(envs[1])
		command += vars_command + " "
	}
	return command
}
func NextApplyDir(path string) (applyDirName string, nextApplyDirError error) {
	if path == "" {
		path = "."
	}
	if path[:1] == "/" {
		path = path[1:]
	}

	dir := path + "/" + ONCTLDIR
	ok, err := exists(dir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(ok)
	fmt.Println(dir)
	// Check if .onctl dir exists
	if ok, err := exists(dir); err != nil {
		log.Fatal(err)
	} else if !ok { // .onctl dir does not exist
		// Create .onctl dir
		fmt.Println("Creating .onctl dir")
		err := os.Mkdir(dir, 0755)
		if err != nil {
			log.Fatal(err)
		}
		// Create apply dir
		applyDirName = dir + "/apply00"
		err = os.Mkdir(applyDirName, 0755)
		if err != nil {
			log.Fatal(err)
		}
		return applyDirName, nil
	} else if ok { // .onctl dir exists
		fmt.Println("onctl exists; Checking apply dir")
		files, err := os.ReadDir(dir)
		if err != nil {
			log.Fatal(err)
		}
		maxNum := -1
		for _, f := range files {
			fmt.Println(f.Name())
			// Extract the number from the directory name
			dirName := f.Name()
			numStr := strings.TrimPrefix(dirName, "apply")
			fmt.Println(numStr)
			num, err := strconv.Atoi(numStr)
			if err == nil && num > maxNum {
				maxNum = num
			}
		}
		applyDirName = path + "/" + ONCTLDIR + "/apply" + fmt.Sprintf("%02d", maxNum+1)
		// Check if apply dir exists
		if okApply, err := exists(applyDirName); err != nil {
			log.Fatal(err)
		} else if !okApply { // apply dir does not exist
			// Create apply dir
			fmt.Println(maxNum)
			err = os.Mkdir(applyDirName, 0755)
			if err != nil {
				log.Fatal(err)

			}
			return applyDirName, nil
		}
	}
	return "", nil
}

func (r *Remote) RemoteRun(remoteRunConfig *RemoteRunConfig) (string, error) {
	log.Println("[DEBUG] remoteRunConfig: ", remoteRunConfig)
	// Create a new SSH connection
	err := r.NewSSHConnection()
	if err != nil {
		return "", err
	}

	// Create a session. It is one session per command.
	session, err := r.Client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() {
		if err := session.Close(); err != nil && err != io.EOF {
			log.Printf("Failed to close session: %v", err)
		}
	}()
	stdOutReader, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}

	// Set env vars
	if len(remoteRunConfig.Vars) > 0 {
		remoteRunConfig.Command = variablesToEnvVars(remoteRunConfig.Vars) + " && " + remoteRunConfig.Command
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

// creates a new apply dir and copies the file to the remote ex. ~/.onctl/apply01
// executes the file on the remote
func (r *Remote) CopyAndRunRemoteFile(config *CopyAndRunRemoteFileConfig) error {
	log.Println("[DEBUG] CopyAndRunRemoteFile: ", config.File)
	fileBaseName := filepath.Base(config.File)
	fileContent, err := files.EmbededFiles.ReadFile("apply_dir.sh")
	if err != nil {
		log.Fatalln(err)
	}
	command := string(fileContent)
	nextApplyDir, err := r.RemoteRun(&RemoteRunConfig{
		Command: command,
		Vars:    []string{"ONCTLDIR=" + ONCTLDIR},
	})
	log.Println("[DEBUG] nextApplyDir: ", nextApplyDir)
	if err != nil {
		fmt.Println(nextApplyDir)
		log.Fatalln(err)
	}
	dstPath := ONCTLDIR + "/" + nextApplyDir + "/" + fileBaseName
	log.Println("[DEBUG] dstPath:", dstPath)
	err = r.SSHCopyFile(config.File, dstPath)
	if err != nil {
		log.Println("RemoteRun error: ", err)
		return err
	}

	config.Vars = append(config.Vars, "PUBLIC_IP="+r.IPAddress)
	command = "cd " + ONCTLDIR + "/" + nextApplyDir + " && chmod +x " + fileBaseName + " && if [[ -f .env ]]; then set -o allexport; source .env; set +o allexport; fi && " + variablesToEnvVars(config.Vars) + "sudo -E ./" + fileBaseName + "> output-" + fileBaseName + ".log 2>&1"

	log.Println("[DEBUG] command: ", command)
	_, err = r.RemoteRun(&RemoteRunConfig{
		Command: command,
	})
	if err != nil {
		return err
	}
	return nil
}
