package tools

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDotEnvFile_Basic(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "dotenv-test-")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Remove(tmpFile.Name())) }()

	content := "KEY1=value1\nKEY2=value2\n# comment\n\nKEY3=value3\n"
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	vars, err := ParseDotEnvFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Equal(t, []string{"KEY1=value1", "KEY2=value2", "KEY3=value3"}, vars)
}

func TestParseDotEnvFile_NonExistent(t *testing.T) {
	_, err := ParseDotEnvFile("/nonexistent/dotenv-file")
	assert.Error(t, err)
}

func TestParseDotEnvFile_SkipsComments(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "dotenv-comments-")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Remove(tmpFile.Name())) }()

	content := "# this is a comment\nFOO=bar\n# another comment\n  # indented comment\nBAZ=qux\n"
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	vars, err := ParseDotEnvFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Equal(t, []string{"FOO=bar", "BAZ=qux"}, vars)
}

func TestParseDotEnvFile_Empty(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "dotenv-empty-")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Remove(tmpFile.Name())) }()
	require.NoError(t, tmpFile.Close())

	vars, err := ParseDotEnvFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Nil(t, vars)
}

func TestExists_ExistingFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "exists-test-")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Remove(tmpFile.Name())) }()
	require.NoError(t, tmpFile.Close())

	ok, err := exists(tmpFile.Name())
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestExists_NonExistent(t *testing.T) {
	ok, err := exists("/nonexistent/path/that/does/not/exist")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestExists_Directory(t *testing.T) {
	tmpDir := t.TempDir()
	ok, err := exists(tmpDir)
	require.NoError(t, err)
	assert.True(t, ok)
}
