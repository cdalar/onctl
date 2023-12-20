package tools

import (
	"bytes"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// e.g. output, err := remoteRun("root", "MY_IP", "PRIVATE_KEY", "ls")
func RemoteRun(user string, addr string, privateKey string, cmd string) (string, error) {
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
	client, err := ssh.Dial("tcp", net.JoinHostPort(addr, "22"), config)
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
