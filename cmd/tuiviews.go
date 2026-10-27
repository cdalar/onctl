package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cdalar/onctl/pkg/cloud"
)

// The tui's resource views, k9s-style: each is a table over one of the
// listings the CLI has (`:vms`, `:images`, `:sizes`) or gh has (`:repos`,
// tuirepos.go), with its own columns, hotkeys and sort keys. A view's rows are computed from the
// model's data every render, so a refresh or a filter never has to
// patch a table.

// tableRow is one line of a view: key finds the thing again (a VM name,
// an image or size name), cells are the columns, sortKeys (if set) sort
// instead of the cells, and state colours the STATE column.
type tableRow struct {
	key      string
	cells    []string
	sortKeys []string
	state    string
}

func (r tableRow) sortKey(col int) string {
	if r.sortKeys != nil && col < len(r.sortKeys) && r.sortKeys[col] != "" {
		return r.sortKeys[col]
	}
	if col < len(r.cells) {
		return r.cells[col]
	}
	return ""
}

// column is a view column: title, and the shift+letter that sorts by it
// (0 for none).
type column struct {
	title string
	sort  rune
}

// hotkey is one entry of the header's legend and the help page.
type hotkey struct{ key, label string }

type tableView struct {
	name     string   // the `:name` that opens it, and its crumb
	aliases  []string // other `:names`
	title    string   // the table's title: "VMs"
	resource string   // the listing it needs: vms, images, sizes, repos; "" for none
	columns  []column
	rows     func(m *tuiModel) []tableRow
	hotkeys  []hotkey // the view's own, before the common ones
	stateCol int      // the column coloured by row.state, -1 for none
	empty    string   // what to say with no rows at all
}

// commonHotkeys apply in every table view.
var commonHotkeys = []hotkey{
	{":", "command"}, {"/", "filter"}, {"?", "help"}, {"esc", "back"}, {"ctrl-c", "quit"},
}

var tuiViews = []*tableView{vmsView, agentsView, imagesView, sizesView, providersView, reposView}

// findView is the view `:name` means, nil for none.
func findView(name string) *tableView {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, v := range tuiViews {
		if v.name == name {
			return v
		}
		for _, a := range v.aliases {
			if a == name {
				return v
			}
		}
	}
	return nil
}

var vmsView = &tableView{
	name:     "vms",
	aliases:  []string{"vm", "servers", "boxes", "v"},
	title:    "VMs",
	resource: "vms",
	columns: []column{
		{"NAME", 'N'}, {"STATE", 'S'}, {"PUBLIC IP", 'I'}, {"TYPE", 'T'}, {"IMAGE", 'M'}, {"LOCATION", 'L'}, {"AGE", 'A'}, {"COST/H", 'C'},
	},
	stateCol: 1,
	empty:    "No VMs yet. Press c to create one.",
	hotkeys: []hotkey{
		{"s", "shell"}, {"a", "claude"}, {"d", "describe"}, {"y", "json"}, {"c", "create"},
		{"p", "pause"}, {"r", "resume"}, {"space", "mark"}, {"ctrl-d", "destroy"},
		{"0", "all"}, {"1", "running"}, {"2", "paused"},
	},
	rows: func(m *tuiModel) []tableRow {
		var rows []tableRow
		for i := range m.vms {
			vm := &m.vms[i]
			if !m.stateFilter.allows(vm.Status) {
				continue
			}
			state := vm.Status
			if v, ok := m.pending[vm.Name]; ok {
				state = presentTense(v)
			}
			rows = append(rows, tableRow{
				key:   vm.Name,
				state: vm.Status,
				cells: []string{vm.Name, state, orDash(vm.IP), orDash(vm.Type), orDash(vm.Image), orDash(vm.Location), vmAge(vm.CreatedAt), costPerHour(*vm)},
				sortKeys: []string{"", strconv.Itoa(vmStateRank(vm.Status)) + vm.Status, "", "", "", "",
					newestFirstKey(vm.CreatedAt), fmt.Sprintf("%016.6f", vm.Cost.CostPerHour)},
			})
		}
		return rows
	},
}

var imagesView = &tableView{
	name:     "images",
	aliases:  []string{"image", "img", "i"},
	title:    "Images",
	resource: "images",
	columns:  []column{{"NAME", 'N'}, {"TYPE", 'T'}, {"OS FLAVOR", 'F'}, {"OS VERSION", 'V'}, {"DESCRIPTION", 'D'}},
	stateCol: -1,
	empty:    "No images available.",
	hotkeys:  []hotkey{{"c", "create VM from image"}},
	rows: func(m *tuiModel) []tableRow {
		var rows []tableRow
		for _, img := range m.images {
			rows = append(rows, tableRow{key: img.Name,
				cells: []string{img.Name, orDash(img.Type), orDash(img.OSFlavor), orDash(img.OSVersion), orDash(img.Description)}})
		}
		return rows
	},
}

var sizesView = &tableView{
	name:     "sizes",
	aliases:  []string{"size", "sz", "types", "type"},
	title:    "Sizes",
	resource: "sizes",
	columns:  []column{{"NAME", 'N'}, {"VCPU", 'V'}, {"MEMORY", 'M'}, {"DISK", 'D'}, {"ARCH", 'A'}, {"PRICE/HOUR", 'H'}, {"PRICE/MONTH", 'P'}, {"DEFAULT", 0}},
	stateCol: -1,
	empty:    "No sizes available.",
	hotkeys:  []hotkey{{"c", "create VM of this size"}},
	rows: func(m *tuiModel) []tableRow {
		var rows []tableRow
		for _, s := range m.sizes {
			vcpu, mem, disk := "-", "-", "-"
			if s.VCPU > 0 {
				vcpu = strconv.Itoa(s.VCPU)
			}
			if s.MemoryMiB > 0 {
				mem = formatMiB(s.MemoryMiB)
			}
			if s.DiskGiB > 0 {
				disk = strconv.Itoa(s.DiskGiB) + " GiB"
			}
			def := ""
			if s.Default {
				def = "yes"
			}
			rows = append(rows, tableRow{
				key:      s.Name,
				cells:    []string{s.Name, vcpu, mem, disk, orDash(s.Arch), orDash(s.HourlyPrice), orDash(s.MonthlyPrice), def},
				sortKeys: []string{"", fmt.Sprintf("%06d", s.VCPU), fmt.Sprintf("%08d", s.MemoryMiB), fmt.Sprintf("%08d", s.DiskGiB), "", "", "", ""},
			})
		}
		return rows
	},
}

// providersView is `:provider` with no name (k9s's `:ctx`): the clouds
// onctl knows, the current one marked; enter switches to the row's.
var providersView = &tableView{
	name:     "providers",
	aliases:  []string{"provider", "ctx", "cloud", "clouds"},
	title:    "Providers",
	resource: "",
	columns:  []column{{"NAME", 'N'}, {"PROVIDER", 'P'}, {"WHERE", 'W'}, {"CURRENT", 0}},
	stateCol: -1,
	empty:    "No providers.",
	hotkeys:  []hotkey{{"enter", "switch to provider"}},
	rows: func(m *tuiModel) []tableRow {
		var rows []tableRow
		for _, name := range cloudProviderList {
			current := ""
			if name == cloudProvider {
				current = "*"
			}
			// The full name and where it runs, as the pick onctl tui
			// starts with shows them (providerChoices).
			p := providerPlaces[name]
			rows = append(rows, tableRow{key: name, cells: []string{name, orDash(p.title), p.kind.offering(), current}})
		}
		return rows
	},
}

// stateFilter is the vms view's 0/1/2 (k9s's namespace keys): every VM,
// the live ones or the paused ones (ls's two sections).
type stateFilter int

const (
	showAllVMs stateFilter = iota
	showLiveVMs
	showPausedVMs
)

func (f stateFilter) allows(status string) bool {
	switch f {
	case showLiveVMs:
		return vmStateRank(status) == 0
	case showPausedVMs:
		return vmStateRank(status) == 1
	}
	return true
}

func (f stateFilter) String() string {
	switch f {
	case showLiveVMs:
		return "running"
	case showPausedVMs:
		return "paused"
	}
	return "all"
}

// sortRows orders rows by col (its sort key), descending if desc; a
// stable sort, so rows the key doesn't tell apart keep the view's own
// order. col -1 leaves them as they are.
func sortRows(rows []tableRow, col int, desc bool) {
	if col < 0 {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].sortKey(col), rows[j].sortKey(col)
		if desc {
			return a > b
		}
		return a < b
	})
}

// filterRows keeps the rows with filter (lowercase) in any cell.
func filterRows(rows []tableRow, filter string) []tableRow {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return rows
	}
	var out []tableRow
	for _, r := range rows {
		for _, c := range r.cells {
			if strings.Contains(strings.ToLower(c), filter) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// describeVM is the `d` page: the VM as `kubectl describe` would lay it
// out, everything the provider says about it.
func describeVM(vm cloud.Vm) []string {
	kv := func(k, v string) string { return fmt.Sprintf("%-16s%s", k+":", v) }
	var lines []string
	lines = append(lines, kv("Name", vm.Name))
	if vm.ID != "" && vm.ID != vm.Name {
		lines = append(lines, kv("ID", vm.ID))
	}
	lines = append(lines, kv("State", vm.Status))
	lines = append(lines, kv("Provider", orDash(vm.Provider)))
	lines = append(lines, kv("Public IP", orDash(vm.IP)))
	lines = append(lines, kv("Private IP", orDash(vm.PrivateIP)))
	lines = append(lines, kv("Type", orDash(vm.Type)))
	lines = append(lines, kv("Image", orDash(vm.Image)))
	if vm.BaseImage != "" {
		lines = append(lines, kv("Base Image", vm.BaseImage))
	}
	lines = append(lines, kv("Location", orDash(vm.Location)))
	if vm.SSHPort > 0 {
		lines = append(lines, kv("SSH Port", strconv.Itoa(vm.SSHPort)))
	}
	if vm.SSHKeyID != "" {
		lines = append(lines, kv("SSH Key ID", vm.SSHKeyID))
	}
	if vm.Provider == "fc" || vm.Provider == "ch" {
		ready := "no"
		if vm.SSHReady {
			ready = "yes"
		}
		lines = append(lines, kv("SSH Ready", ready))
	}
	if !vm.CreatedAt.IsZero() {
		lines = append(lines, kv("Created", stamp(vm.CreatedAt)+" ("+vmAge(vm.CreatedAt)+" ago)"))
	}
	if c := vm.Cost; c.CostPerHour > 0 || c.CostPerMonth > 0 || c.AccumulatedCost > 0 {
		lines = append(lines, "", "Cost:")
		lines = append(lines, "  "+kv("Per Hour", fmt.Sprintf("%.4f %s", c.CostPerHour, c.Currency)))
		lines = append(lines, "  "+kv("Per Month", fmt.Sprintf("%.2f %s", c.CostPerMonth, c.Currency)))
		lines = append(lines, "  "+kv("Accumulated", fmt.Sprintf("%.4f %s", c.AccumulatedCost, c.Currency)))
	}
	return lines
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// vmAge is ls's AGE column; "-" when the provider didn't say.
func vmAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return durationFromCreatedAt(t)
}

// costPerHour is ls's COST/H column; "-" when the provider has no price.
func costPerHour(vm cloud.Vm) string {
	if vm.Cost.CostPerHour == 0 {
		return "-"
	}
	return fmt.Sprintf("%.4f%s", vm.Cost.CostPerHour, vm.Cost.Currency)
}

// newestFirstKey sorts ascending by AGE as newest first, like ls.
func newestFirstKey(t time.Time) string {
	return fmt.Sprintf("%020d", -t.Unix()+1<<62)
}

func stamp(t time.Time) string { return t.Local().Format(time.DateTime) }
