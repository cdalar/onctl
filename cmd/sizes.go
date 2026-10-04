package cmd

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/cdalar/onctl/pkg/cloud"
	"github.com/spf13/cobra"
)

var sizesCmd = &cobra.Command{
	Use:   "sizes",
	Short: "List the VM sizes the current provider offers (what --type takes)",
	Long: `Lists the VM sizes --type accepts for the current provider, with vCPUs,
memory and disk -- and, where the provider publishes them, prices for the
configured location. The default size is marked with *.`,
	Example: `  onctl sizes -p hetzner
  onctl sizes -p boxes`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		switch cloudProvider {
		case "fc", "ch":
			fmt.Println("The " + cloudProvider + " provider has no fixed sizes: set --vcpu and --memory on create.")
			return
		}
		lister, ok := provider.(cloud.SizeLister)
		if !ok {
			fmt.Println("The current cloud provider does not support listing sizes yet.")
			return
		}
		sizes, err := lister.ListSizes()
		if err != nil {
			log.Fatalln(err)
		}
		rows := make([]sizeRow, 0, len(sizes))
		for _, s := range sizes {
			rows = append(rows, newSizeRow(s))
		}
		TabWriter(rows, sizesTemplate(sizes))
	},
}

// sizeRow is a CloudSize formatted for the table.
type sizeRow struct {
	Name, VCPU, Memory, Disk, Arch, Hourly, Monthly string
}

func newSizeRow(s cloud.CloudSize) sizeRow {
	name := s.Name
	if s.Default {
		name += " *"
	}
	r := sizeRow{Name: name, Arch: s.Arch, Hourly: s.HourlyPrice, Monthly: s.MonthlyPrice}
	if s.VCPU > 0 {
		r.VCPU = strconv.Itoa(s.VCPU)
	}
	if s.MemoryMiB > 0 {
		r.Memory = formatMiB(s.MemoryMiB)
	}
	if s.DiskGiB > 0 {
		r.Disk = strconv.Itoa(s.DiskGiB) + " GiB"
	}
	return r
}

// formatMiB shows whole GiB as GiB, anything else as MiB.
func formatMiB(mib int) string {
	if mib%1024 == 0 {
		return strconv.Itoa(mib/1024) + " GiB"
	}
	if mib > 1024 {
		return strconv.FormatFloat(float64(mib)/1024, 'f', 1, 64) + " GiB"
	}
	return strconv.Itoa(mib) + " MiB"
}

// sizesTemplate is the sizes table, without the columns no size fills in
// (prices for providers that don't publish them, say).
func sizesTemplate(sizes []cloud.CloudSize) string {
	cols := []struct {
		header, field string
		has           func(cloud.CloudSize) bool
	}{
		{"NAME", "Name", func(cloud.CloudSize) bool { return true }},
		{"VCPU", "VCPU", func(s cloud.CloudSize) bool { return s.VCPU > 0 }},
		{"MEMORY", "Memory", func(s cloud.CloudSize) bool { return s.MemoryMiB > 0 }},
		{"DISK", "Disk", func(s cloud.CloudSize) bool { return s.DiskGiB > 0 }},
		{"ARCH", "Arch", func(s cloud.CloudSize) bool { return s.Arch != "" }},
		{"PRICE/HOUR", "Hourly", func(s cloud.CloudSize) bool { return s.HourlyPrice != "" }},
		{"PRICE/MONTH", "Monthly", func(s cloud.CloudSize) bool { return s.MonthlyPrice != "" }},
	}
	var headers, fields []string
	for _, c := range cols {
		for _, s := range sizes {
			if c.has(s) {
				headers = append(headers, c.header)
				fields = append(fields, "{{."+c.field+"}}")
				break
			}
		}
	}
	return strings.Join(headers, "\t") + "\n{{range .}}" + strings.Join(fields, "\t") + "\n{{end}}"
}

// completeSizes completes --type with the provider's sizes, when it can
// list them.
func completeSizes(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ensureProvider()
	lister, ok := provider.(cloud.SizeLister)
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	sizes, err := lister.ListSizes()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string
	for _, s := range sizes {
		names = append(names, s.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	rootCmd.AddCommand(sizesCmd)
	_ = createCmd.RegisterFlagCompletionFunc("type", completeSizes)
}
