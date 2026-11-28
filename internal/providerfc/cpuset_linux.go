package providerfc

import "golang.org/x/sys/unix"

// pinCPUs sets pid's CPU affinity to cpuset (taskset -c's list form).
func pinCPUs(pid int, cpuset string) error {
	cpus, err := parseCPUSet(cpuset)
	if err != nil {
		return err
	}
	var set unix.CPUSet
	for _, c := range cpus {
		set.Set(c)
	}
	return unix.SchedSetaffinity(pid, &set)
}
