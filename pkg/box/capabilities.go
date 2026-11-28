package box

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/cdalar/onctl/internal/providerfc"
	"github.com/cdalar/onctl/pkg/cloud"
)

// ErrUnsupported is what a capability returns, wrapped, on a provider
// that doesn't have it (sizes on fc, restart anywhere but fc, ...).
var ErrUnsupported = errors.New("not supported by this provider")

func (c *Client) unsupported(what string) error {
	return fmt.Errorf("%s: %s: %w", c.id, what, ErrUnsupported)
}

// fcCatalogURL is where fc's images are listed from; tests point it at a
// fake.
var fcCatalogURL = providerfc.PublicCatalogURL

// Images is the OS images Create takes (Vm.Image) on this provider. fc's
// are its public catalog, which needs nothing installed to list.
func (c *Client) Images() ([]cloud.CloudImage, error) {
	if c.id == "fc" {
		return providerfc.CatalogLister{BaseURL: fcCatalogURL}.ListImages()
	}
	if l, ok := c.p.(cloud.ImageLister); ok {
		return l.ListImages()
	}
	return nil, c.unsupported("listing images")
}

// Sizes is the VM sizes Create takes (Vm.Type) on this provider. fc, ch
// and ac have none: their VMs are sized by vCPU and memory.
func (c *Client) Sizes() ([]cloud.CloudSize, error) {
	if l, ok := c.p.(cloud.SizeLister); ok {
		return l.ListSizes()
	}
	return nil, c.unsupported("listing sizes")
}

// CanRestart reports whether Restart works on this provider (fc only).
func (c *Client) CanRestart() bool {
	_, ok := c.p.(cloud.Restarter)
	return ok
}

// Restart cold-boots the VM named name from its own disk; see
// cloud.Restarter.
func (c *Client) Restart(name string, opts cloud.RestartOptions) (cloud.Vm, error) {
	r, ok := c.p.(cloud.Restarter)
	if !ok {
		return cloud.Vm{}, c.unsupported("restart")
	}
	return r.Restart(cloud.Vm{Name: name}, opts)
}

// Tunneled reports whether this provider's VMs have no address of their
// own (boxes, openshell): Dial goes through the provider, and a VM's IP
// isn't something to show or connect to.
func (c *Client) Tunneled() bool {
	_, ok := c.p.(cloud.Dialer)
	return ok
}

// Dial connects to port on vm: through the provider when Tunneled,
// otherwise over TCP to its IP (looked up by name when vm has none).
func (c *Client) Dial(ctx context.Context, vm cloud.Vm, port int) (net.Conn, error) {
	if d, ok := c.p.(cloud.Dialer); ok {
		return d.DialVM(ctx, vm, port)
	}
	if vm.IP == "" {
		got, err := c.Get(vm.Name)
		if err != nil {
			return nil, err
		}
		if got.IP == "" {
			return nil, fmt.Errorf("%s has no address to connect to", vm.Name)
		}
		vm = got
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", net.JoinHostPort(vm.IP, strconv.Itoa(port)))
}

// AuthorizeKey puts publicKey on vm, on providers that do that for an
// existing VM themselves (boxes). Elsewhere the key goes on at Create and
// Resume (AddSSHKey), and this returns ErrUnsupported.
func (c *Client) AuthorizeKey(vm cloud.Vm, publicKey string) error {
	ka, ok := c.p.(cloud.KeyAuthorizer)
	if !ok {
		return c.unsupported("authorizing a key")
	}
	return ka.AuthorizeKey(vm, publicKey)
}
