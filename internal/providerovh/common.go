package providerovh

import (
	"github.com/ovh/go-ovh/ovh"
)

// GetClient builds an OVHcloud API client. Unlike Hetzner's single
// HCLOUD_TOKEN, OVH requires an application key/secret plus a consumer key
// obtained once out-of-band (see https://api.ovh.com/createToken/), so this
// follows the aws/gcp/azure convention of letting the SDK's own credential
// chain (OVH_ENDPOINT, OVH_APPLICATION_KEY, OVH_APPLICATION_SECRET,
// OVH_CONSUMER_KEY env vars, or an ovh.conf file) resolve everything, and
// returning its error if it can't.
func GetClient() (*ovh.Client, error) {
	return ovh.NewDefaultClient()
}
