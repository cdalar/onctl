package cmd

import "strings"

// Where a provider makes its machines, for the messages that would
// otherwise say just "on ac": the name behind the short one, and whether
// that is this computer, a public cloud, or somewhere else.
type providerPlace struct {
	name string
	kind placeKind
}

type placeKind int

const (
	placeLocal  placeKind = iota // made on the machine onctl runs on
	placeCloud                   // a public cloud
	placeRemote                  // elsewhere, not a cloud of its own
)

var providerPlaces = map[string]providerPlace{
	"aws":     {"Amazon Web Services", placeCloud},
	"hetzner": {"Hetzner Cloud", placeCloud},
	"azure":   {"Microsoft Azure", placeCloud},
	"gcp":     {"Google Cloud", placeCloud},
	"ovh":     {"OVHcloud", placeCloud},
	"fc":      {"Firecracker microVM", placeLocal},
	"ch":      {"Cloud Hypervisor microVM", placeLocal},
	"ac":      {"Apple container", placeLocal},
	"static":  {"a static host from onctl.yaml", placeRemote},
	"boxes":   {"the boxes service", placeRemote},
}

// creatingMessage says what is being made for name, and where.
func creatingMessage(name string) string {
	return "Creating " + name + " " + providerWhere() + "..."
}

// providerWhere is where the selected provider puts a machine, as a
// phrase after the machine's name: "as an Apple container on this
// machine", "in the cloud on Hetzner Cloud", "on the boxes service".
func providerWhere() string {
	p, ok := providerPlaces[cloudProvider]
	if !ok {
		return "on " + cloudProvider
	}
	switch p.kind {
	case placeLocal:
		article := "a"
		if strings.ContainsRune("aeiouAEIOU", rune(p.name[0])) {
			article = "an"
		}
		return "as " + article + " " + p.name + " on this machine"
	case placeCloud:
		return "in the cloud on " + p.name
	}
	return "on " + p.name
}
