package providerfc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The boxctl server hands out CPU sets in taskset -c's list form; a
// typo must fail the create rather than pin the VM somewhere else.
func TestParseCPUSet(t *testing.T) {
	cpus, err := parseCPUSet("0-3,8, 10-11")
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2, 3, 8, 10, 11}, cpus)
	for _, bad := range []string{"", "a", "3-1", "-1", "1-", "1,,2"} {
		_, err := parseCPUSet(bad)
		assert.Error(t, err, bad)
	}
}
