package tools

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExists_FileExists(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "exists-test-*")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	ok, err := exists(tmpFile.Name())
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestExists_FileNotExists(t *testing.T) {
	ok, err := exists("/nonexistent/path/that/does/not/exist")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestExists_Directory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "exists-dir-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ok, err := exists(tmpDir)
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestParseDotEnvFile_ValidFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "dotenv-test-*.env")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := `# This is a comment
KEY1=value1
KEY2=value2

# Another comment
KEY3=value with spaces
`
	_, err = tmpFile.WriteString(content)
	assert.NoError(t, err)
	tmpFile.Close()

	vars, err := ParseDotEnvFile(tmpFile.Name())
	assert.NoError(t, err)
	assert.Equal(t, []string{"KEY1=value1", "KEY2=value2", "KEY3=value with spaces"}, vars)
}

func TestParseDotEnvFile_NonExistentFile(t *testing.T) {
	vars, err := ParseDotEnvFile("/nonexistent/file.env")
	assert.Error(t, err)
	assert.Nil(t, vars)
}

func TestParseDotEnvFile_EmptyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "dotenv-empty-*.env")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	vars, err := ParseDotEnvFile(tmpFile.Name())
	assert.NoError(t, err)
	assert.Nil(t, vars)
}

func TestParseDotEnvFile_OnlyComments(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "dotenv-comments-*.env")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("# just a comment\n# another comment\n")
	assert.NoError(t, err)
	tmpFile.Close()

	vars, err := ParseDotEnvFile(tmpFile.Name())
	assert.NoError(t, err)
	assert.Nil(t, vars)
}

func TestNextApplyDir_NewDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nextapply-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	origDir, err := os.Getwd()
	assert.NoError(t, err)
	defer os.Chdir(origDir)

	err = os.Chdir(tmpDir)
	assert.NoError(t, err)

	// Create a relative subdir to work with (NextApplyDir strips leading "/" making absolute paths relative)
	err = os.MkdirAll("workdir", 0755)
	assert.NoError(t, err)

	applyDir, err := NextApplyDir("workdir")
	assert.NoError(t, err)
	assert.NotEmpty(t, applyDir)
	assert.Contains(t, applyDir, "apply")
}

func TestNextApplyDir_ExistingApplyDirs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nextapply-existing-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	origDir, err := os.Getwd()
	assert.NoError(t, err)
	defer os.Chdir(origDir)

	err = os.Chdir(tmpDir)
	assert.NoError(t, err)

	// Create workdir with existing .onctl/apply00
	err = os.MkdirAll("workdir/.onctl/apply00", 0755)
	assert.NoError(t, err)

	applyDir, err := NextApplyDir("workdir")
	assert.NoError(t, err)
	assert.NotEmpty(t, applyDir)
	assert.Contains(t, applyDir, "apply01")
}

func TestNextApplyDir_EmptyPath(t *testing.T) {
	// With empty path it uses "." which creates .onctl in current dir
	// We need to work in a temp dir
	tmpDir, err := os.MkdirTemp("", "nextapply-empty-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	origDir, err := os.Getwd()
	assert.NoError(t, err)
	defer os.Chdir(origDir)

	err = os.Chdir(tmpDir)
	assert.NoError(t, err)

	applyDir, err := NextApplyDir("")
	assert.NoError(t, err)
	assert.NotEmpty(t, applyDir)
}
