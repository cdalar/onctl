package providerfc

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCPUSet is taskset -c's list form ("0-3,8") as CPU numbers.
func parseCPUSet(s string) ([]int, error) {
	var cpus []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		lo, hi, isRange := strings.Cut(part, "-")
		first, err := strconv.Atoi(lo)
		if err != nil || first < 0 {
			return nil, fmt.Errorf("invalid CPU list %q", s)
		}
		last := first
		if isRange {
			if last, err = strconv.Atoi(hi); err != nil || last < first {
				return nil, fmt.Errorf("invalid CPU list %q", s)
			}
		}
		for c := first; c <= last; c++ {
			cpus = append(cpus, c)
		}
	}
	return cpus, nil
}
