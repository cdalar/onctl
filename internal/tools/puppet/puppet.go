package puppet

type Inventory struct {
	Groups []Group `yaml:"groups"`
	Config Config  `yaml:"config"`
}

type Group struct {
	Name    string   `yaml:"name"`
	Targets []string `yaml:"targets"`
	// Config  Config   `yaml:"config"`
}

type Config struct {
	Transport string `yaml:"transport"`
	SSH       SSH    `yaml:"ssh"`
}

type SSH struct {
	User         string `yaml:"user,omitempty"`
	Password     string `yaml:"password,omitempty"`    // Consider using SSH keys instead
	PrivateKey   string `yaml:"private-key,omitempty"` // Uncomment if using SSH keys
	HostKeyCheck bool   `yaml:"host-key-check"`        // Optional: set to false to disable host key checking
	RunAs        string `yaml:"run-as,omitempty"`      // Optional: specify a user to escalate to
	NativeSSH    bool   `yaml:"native-ssh"`            // Optional: set to true to use native ssh instead of go ssh
	SSHCommand   string `yaml:"ssh-command,omitempty"` // Optional: specify a custom ssh command
}
