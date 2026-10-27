package cmd

import "strings"

// Where a provider makes its machines, for the messages that would
// otherwise say just "on ac": the name behind the short one, and whether
// that is this computer, a public cloud, or somewhere else. title is the
// provider's full name on its own, for lists like the tui's provider pick.
type providerPlace struct {
	name  string
	kind  placeKind
	title string
}

type placeKind int

const (
	placeLocal  placeKind = iota // made on the machine onctl runs on
	placeCloud                   // a public cloud
	placeRemote                  // elsewhere, not a cloud of its own
	placeSaaS                    // a hosted service that runs the machines for you
)

// offering is how the tui's provider pick labels a kind: on this
// machine, in a cloud account of your own, a SaaS, or a host you bring.
func (k placeKind) offering() string {
	switch k {
	case placeLocal:
		return "local"
	case placeCloud:
		return "cloud, BYO account"
	case placeSaaS:
		return "cloud, SaaS"
	}
	return "remote, BYO host"
}

var providerPlaces = map[string]providerPlace{
	"aws":     {"Amazon Web Services", placeCloud, "Amazon Web Services"},
	"hetzner": {"Hetzner Cloud", placeCloud, "Hetzner Cloud"},
	"azure":   {"Microsoft Azure", placeCloud, "Microsoft Azure"},
	"gcp":     {"Google Cloud", placeCloud, "Google Cloud"},
	"ovh":     {"OVHcloud", placeCloud, "OVHcloud"},
	"fc":      {"Firecracker microVM", placeLocal, "Firecracker microVM"},
	"ch":      {"Cloud Hypervisor microVM", placeLocal, "Cloud Hypervisor microVM"},
	"ac":      {"Apple container", placeLocal, "Apple Container"},
	"static":  {"a static host from onctl.yaml", placeRemote, "Static host (onctl.yaml)"},
	"boxes":   {"the boxes service", placeSaaS, "Boxes service"},
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
