package cmd

import (
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/stretchr/testify/assert"
)

func TestSizesTemplateDropsEmptyColumns(t *testing.T) {
	boxes := []cloud.CloudSize{{Name: "small", VCPU: 1, MemoryMiB: 1024, DiskGiB: 10}}
	assert.Equal(t, "NAME\tVCPU\tMEMORY\tDISK\n{{range .}}{{.Name}}\t{{.VCPU}}\t{{.Memory}}\t{{.Disk}}\n{{end}}", sizesTemplate(boxes))

	hetzner := []cloud.CloudSize{{Name: "cpx21", VCPU: 3, MemoryMiB: 4096, DiskGiB: 80, Arch: "x86", HourlyPrice: "0.0080 EUR", MonthlyPrice: "4.50 EUR"}}
	assert.Equal(t, "NAME\tVCPU\tMEMORY\tDISK\tARCH\tPRICE/HOUR\tPRICE/MONTH\n{{range .}}{{.Name}}\t{{.VCPU}}\t{{.Memory}}\t{{.Disk}}\t{{.Arch}}\t{{.Hourly}}\t{{.Monthly}}\n{{end}}", sizesTemplate(hetzner))
}

func TestNewSizeRow(t *testing.T) {
	r := newSizeRow(cloud.CloudSize{Name: "medium", VCPU: 2, MemoryMiB: 4096, DiskGiB: 20, Default: true})
	assert.Equal(t, sizeRow{Name: "medium *", VCPU: "2", Memory: "4 GiB", Disk: "20 GiB"}, r)

	assert.Equal(t, "512 MiB", formatMiB(512))
	assert.Equal(t, "3.9 GiB", formatMiB(4000), "OVH's 4000 MB isn't a whole GiB")
	assert.Equal(t, "8 GiB", formatMiB(8192))
}
