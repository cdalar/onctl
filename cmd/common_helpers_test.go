package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/cdalar/onctl/internal/cloud"
	"github.com/stretchr/testify/assert"
)

func TestGetNameFromTags_Found(t *testing.T) {
	tags := []*ec2.Tag{
		{Key: aws.String("Env"), Value: aws.String("prod")},
		{Key: aws.String("Name"), Value: aws.String("my-instance")},
		{Key: aws.String("Owner"), Value: aws.String("team")},
	}
	result := getNameFromTags(tags)
	assert.Equal(t, "my-instance", result)
}

func TestGetNameFromTags_NotFound(t *testing.T) {
	tags := []*ec2.Tag{
		{Key: aws.String("Env"), Value: aws.String("prod")},
		{Key: aws.String("Owner"), Value: aws.String("team")},
	}
	result := getNameFromTags(tags)
	assert.Equal(t, "", result)
}

func TestGetNameFromTags_EmptyTags(t *testing.T) {
	result := getNameFromTags([]*ec2.Tag{})
	assert.Equal(t, "", result)
}

func TestDurationFromCreatedAt_Recent(t *testing.T) {
	recent := time.Now().Add(-5 * time.Minute)
	result := durationFromCreatedAt(recent)
	assert.NotEmpty(t, result)
	// k8s duration format: "5m", "1h", etc.
	assert.Regexp(t, `^\d+m$|^\d+h`, result)
}

func TestDurationFromCreatedAt_OldDate(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour)
	result := durationFromCreatedAt(old)
	assert.NotEmpty(t, result)
	// k8s duration format: "2d"
	assert.Regexp(t, `^\d+d$`, result)
}

func TestPrettyPrint_ValidStruct(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	type testData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := testData{Name: "Alice", Age: 30}
	err := PrettyPrint(data)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)
	assert.Contains(t, output, "Alice")
	assert.Contains(t, output, "30")

	// Verify it's valid JSON
	var parsed map[string]interface{}
	assert.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed))
}

func TestPrettyPrint_Map(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	data := map[string]string{"key": "value"}
	err := PrettyPrint(data)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.NoError(t, err)
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
}

func TestGenerateIDToken(t *testing.T) {
	id := GenerateIDToken()
	assert.NotEqual(t, [16]byte{}, id)
	idStr := id.String()
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, idStr)
}

func TestGenerateIDToken_Unique(t *testing.T) {
	id1 := GenerateIDToken()
	id2 := GenerateIDToken()
	assert.NotEqual(t, id1, id2)
}

func TestMergeConfig_EmptyOpt(t *testing.T) {
	// SSHPort defaults to 22 to trigger the merge condition in MergeConfig
	opt := &cmdCreateOptions{Vm: cloud.Vm{SSHPort: 22}}
	config := &cmdCreateOptions{
		PublicKeyFile: "~/.ssh/id_rsa.pub",
		ApplyFiles:    []string{"docker.sh"},
		DotEnvFile:    ".env",
		Variables:     []string{"KEY=val"},
		Domain:        "example.com",
		DownloadFiles: []string{"remote.txt"},
		UploadFiles:   []string{"local.txt"},
		Vm: cloud.Vm{
			Name:          "test-vm",
			Type:          "cx11",
			SSHPort:       2222,
			CloudInitFile: "cloud-init.yaml",
		},
	}

	MergeConfig(opt, config)

	assert.Equal(t, "~/.ssh/id_rsa.pub", opt.PublicKeyFile)
	assert.Equal(t, []string{"docker.sh"}, opt.ApplyFiles)
	assert.Equal(t, ".env", opt.DotEnvFile)
	assert.Equal(t, []string{"KEY=val"}, opt.Variables)
	assert.Equal(t, "test-vm", opt.Vm.Name)
	assert.Equal(t, "cx11", opt.Vm.Type)
	assert.Equal(t, 2222, opt.Vm.SSHPort)
	assert.Equal(t, "cloud-init.yaml", opt.Vm.CloudInitFile)
	assert.Equal(t, "example.com", opt.Domain)
	assert.Equal(t, []string{"remote.txt"}, opt.DownloadFiles)
	assert.Equal(t, []string{"local.txt"}, opt.UploadFiles)
}

func TestMergeConfig_OptTakesPrecedence(t *testing.T) {
	opt := &cmdCreateOptions{
		PublicKeyFile: "custom.pub",
		ApplyFiles:    []string{"custom.sh"},
		DotEnvFile:    "custom.env",
		Variables:     []string{"CUSTOM=1"},
		Domain:        "custom.com",
		Vm: cloud.Vm{
			Name:    "custom-vm",
			Type:    "custom-type",
			SSHPort: 22, // default - should be replaced
		},
	}
	config := &cmdCreateOptions{
		PublicKeyFile: "default.pub",
		ApplyFiles:    []string{"default.sh"},
		DotEnvFile:    "default.env",
		Variables:     []string{"DEFAULT=1"},
		Domain:        "default.com",
		Vm: cloud.Vm{
			Name:    "default-vm",
			Type:    "default-type",
			SSHPort: 2222,
		},
	}

	MergeConfig(opt, config)

	// Opt values should be preserved
	assert.Equal(t, "custom.pub", opt.PublicKeyFile)
	assert.Equal(t, []string{"custom.sh"}, opt.ApplyFiles)
	assert.Equal(t, "custom.env", opt.DotEnvFile)
	assert.Equal(t, []string{"CUSTOM=1"}, opt.Variables)
	assert.Equal(t, "custom-vm", opt.Vm.Name)
	assert.Equal(t, "custom-type", opt.Vm.Type)
	assert.Equal(t, "custom.com", opt.Domain)
}

func TestMergeConfig_DefaultSSHPortReplaced(t *testing.T) {
	opt := &cmdCreateOptions{
		Vm: cloud.Vm{SSHPort: 22}, // default port
	}
	config := &cmdCreateOptions{
		Vm: cloud.Vm{SSHPort: 2222},
	}
	MergeConfig(opt, config)
	assert.Equal(t, 2222, opt.Vm.SSHPort)
}

func TestMergeConfig_NonDefaultSSHPortKept(t *testing.T) {
	opt := &cmdCreateOptions{
		Vm: cloud.Vm{SSHPort: 8022},
	}
	config := &cmdCreateOptions{
		Vm: cloud.Vm{SSHPort: 2222},
	}
	MergeConfig(opt, config)
	assert.Equal(t, 8022, opt.Vm.SSHPort)
}

func TestGetSSHKeyFilePaths_PublicKeyExtension(t *testing.T) {
	pub, priv := getSSHKeyFilePaths("/home/user/.ssh/id_rsa.pub")
	assert.Equal(t, "/home/user/.ssh/id_rsa.pub", pub)
	assert.Equal(t, "/home/user/.ssh/id_rsa", priv)
}

func TestGetSSHKeyFilePaths_PrivateKeyExtension(t *testing.T) {
	pub, priv := getSSHKeyFilePaths("/home/user/.ssh/id_ed25519")
	assert.Equal(t, "/home/user/.ssh/id_ed25519.pub", pub)
	assert.Equal(t, "/home/user/.ssh/id_ed25519", priv)
}

func TestGetSSHKeyFilePaths_TildeExpansion(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	pub, priv := getSSHKeyFilePaths("~/.ssh/id_rsa.pub")
	assert.Equal(t, fmt.Sprintf("%s/.ssh/id_rsa.pub", homeDir), pub)
	assert.Equal(t, fmt.Sprintf("%s/.ssh/id_rsa", homeDir), priv)
}

func TestParseConfigFile_ValidFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-test-*.yaml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := `publicKeyFile: ~/.ssh/id_rsa.pub
applyFiles:
  - docker.sh
dotEnvFile: .env
variables:
  - KEY=value
vm:
  name: test-vm
  type: cx11
  sshPort: 22
domain: example.com
`
	_, err = tmpFile.WriteString(content)
	assert.NoError(t, err)
	tmpFile.Close()

	config, err := parseConfigFile(tmpFile.Name())
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "~/.ssh/id_rsa.pub", config.PublicKeyFile)
	assert.Equal(t, []string{"docker.sh"}, config.ApplyFiles)
	assert.Equal(t, ".env", config.DotEnvFile)
	assert.Equal(t, "test-vm", config.Vm.Name)
	assert.Equal(t, "cx11", config.Vm.Type)
	assert.Equal(t, "example.com", config.Domain)
}

func TestParseConfigFile_NonExistentFile(t *testing.T) {
	config, err := parseConfigFile("/nonexistent/config.yaml")
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "failed to open config file")
}

func TestParseConfigFile_InvalidYAML(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-invalid-*.yaml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("this: is: not: valid: yaml: !!!")
	assert.NoError(t, err)
	tmpFile.Close()

	config, err := parseConfigFile(tmpFile.Name())
	assert.Error(t, err)
	assert.Nil(t, config)
}

func TestFindFile_EmptySlice(t *testing.T) {
	result := findFile([]string{})
	assert.Nil(t, result)
}

func TestFindFile_ExistingFiles(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "findfile-test-*")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	result := findFile([]string{tmpFile.Name()})
	assert.Len(t, result, 1)
	assert.Equal(t, tmpFile.Name(), result[0])
}

func TestFindSingleFile_Empty(t *testing.T) {
	result := findSingleFile("")
	assert.Equal(t, "", result)
}

func TestFindSingleFile_ExistingFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "findsingle-test-*")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	result := findSingleFile(tmpFile.Name())
	assert.Equal(t, tmpFile.Name(), result)
}

func TestTabWriter_SimpleTemplate(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	type Row struct {
		Name string
		Val  string
	}
	data := []Row{{Name: "alice", Val: "foo"}, {Name: "bob", Val: "bar"}}
	TabWriter(data, `{{range .}}{{.Name}}\t{{.Val}}
{{end}}`)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "alice")
	assert.Contains(t, output, "bob")
}


func TestEnsureCursorVisible(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ensureCursorVisible()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Equal(t, "\033[?25h", output)
}
