package box

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/cdalar/onctl/internal/domain"
	"github.com/cdalar/onctl/pkg/cloud"
)

// Transfer is one file or directory copy between this machine and a VM.
type Transfer struct {
	Local, Remote string
}

// Setup is what Provision does to a VM once Create has it, in this order:
// point Domain at it, wait until it's ready, Uploads, ApplyFiles, then
// Downloads.
type Setup struct {
	// Domain, when set, is a Cloudflare DNS name (CLOUDFLARE_API_TOKEN,
	// CLOUDFLARE_ZONE_ID) pointed at the VM's IP.
	Domain string
	// ReadyTimeout is how long to wait for the VM: for cloud-init to
	// finish where the provider runs it, else for ssh. 0 skips the wait.
	ReadyTimeout time.Duration
	Uploads      []Transfer
	// ApplyFiles are scripts run on the VM as root, each in its own apply
	// dir (Conn.Apply), with Vars (NAME=value) in their environment.
	ApplyFiles []string
	Vars       []string
	Downloads  []Transfer
	// Out gets the apply files' output; nil discards it.
	Out io.Writer
	// Progress, when set, hears each step start (Done false) and end.
	Progress func(Step)
	// UploadProgress, when set, hears an upload's bytes copied so far.
	UploadProgress func(t Transfer, current, total int64)
}

// StepKind is a part of Provision.
type StepKind string

const (
	StepDomain   StepKind = "domain"
	StepReady    StepKind = "ready"
	StepUpload   StepKind = "upload"
	StepApply    StepKind = "apply"
	StepDownload StepKind = "download"
)

// Step is one part of Provision starting or ending.
type Step struct {
	Kind StepKind
	// Item is the domain, the apply file or the transfer's local path.
	Item     string
	Transfer Transfer
	Done     bool
	// Err is a finished step's failure.
	Err error
}

// cloudInitProviders are the providers whose VMs run cloud-init, so
// ready means cloud-init finished; elsewhere it means ssh answers.
var cloudInitProviders = map[string]bool{"aws": true, "azure": true, "gcp": true, "hetzner": true, "ovh": true}

// imageProviders are the providers whose Create takes Vm.Image.
var imageProviders = map[string]bool{"hetzner": true, "aws": true, "ovh": true, "boxes": true, "ac": true, "openshell": true}

// CheckSetup is what Create and Provision would refuse for vm and s on
// this provider, before anything is made: an image where it can't be
// chosen, an fc/ch microVM on a host without KVM, a domain for a VM with
// no public IP, files for an OpenShell sandbox, a domain without
// Cloudflare's settings.
func (c *Client) CheckSetup(vm cloud.Vm, s Setup) error {
	if vm.Image != "" && !imageProviders[c.id] {
		return fmt.Errorf("an image can only be chosen on the hetzner, aws, ovh, boxes, ac and openshell providers (this is %s)", c.id)
	}
	if err := localVMMCheck(c.id, runtime.GOOS); err != nil {
		return err
	}
	if c.id == "openshell" && (len(s.ApplyFiles) > 0 || len(s.Uploads) > 0 || len(s.Downloads) > 0 || vm.CloudInitFile != "") {
		return cloud.ErrOpenShellApplyFiles
	}
	if s.Domain != "" {
		if c.Tunneled() {
			return fmt.Errorf("a domain points a DNS record at the VM's public IP, and a %s VM has none", c.id)
		}
		if err := domain.NewCloudFlareService().CheckEnv(); err != nil {
			return fmt.Errorf("domain: %w", err)
		}
	}
	return nil
}

// Provision does s to the VM conn reaches. Not becoming ready ends it
// with that error; any other step's failure is reported (Step.Err) and
// the rest still run, and all of them come back joined.
func (c *Client) Provision(conn *Conn, s Setup) error {
	report := func(st Step) {
		if s.Progress != nil {
			s.Progress(st)
		}
	}
	out := s.Out
	if out == nil {
		out = io.Discard
	}
	var errs []error
	step := func(st Step, f func() error) {
		report(st)
		st.Done, st.Err = true, f()
		if st.Err != nil {
			errs = append(errs, fmt.Errorf("%s %s: %w", st.Kind, st.Item, st.Err))
		}
		report(st)
	}

	vm := conn.VM()
	if s.Domain != "" {
		step(Step{Kind: StepDomain, Item: s.Domain}, func() error {
			_, err := domain.NewCloudFlareService().SetRecord(&domain.SetRecordRequest{Subdomain: s.Domain, Ipaddress: vm.IP})
			return err
		})
	}
	if s.ReadyTimeout > 0 {
		ready := Step{Kind: StepReady, Item: vm.Name}
		report(ready)
		if cloudInitProviders[c.id] {
			ready.Err = conn.WaitForCloudInit(s.ReadyTimeout)
		} else {
			ready.Err = conn.WaitForSSH(s.ReadyTimeout)
		}
		ready.Done = true
		report(ready)
		if ready.Err != nil {
			return errors.Join(append(errs, ready.Err)...)
		}
	}
	for _, t := range s.Uploads {
		var progress func(current, total int64)
		if s.UploadProgress != nil {
			progress = func(current, total int64) { s.UploadProgress(t, current, total) }
		}
		step(Step{Kind: StepUpload, Item: t.Local, Transfer: t}, func() error {
			return conn.Upload(t.Local, t.Remote, progress)
		})
	}
	for _, f := range s.ApplyFiles {
		step(Step{Kind: StepApply, Item: f}, func() error {
			return conn.Apply(f, s.Vars, out)
		})
	}
	if len(s.Downloads) > 0 {
		if err := conn.Open(); err != nil {
			return errors.Join(append(errs, err)...)
		}
	}
	for _, t := range s.Downloads {
		step(Step{Kind: StepDownload, Item: t.Remote, Transfer: t}, func() error {
			return conn.Download(t.Remote, t.Local)
		})
	}
	return errors.Join(errs...)
}

// localVMMCheck refuses an fc or ch VM on a host that can't run one.
// Firecracker and Cloud Hypervisor are KVM virtual machine monitors, so
// they run on Linux only; without this a create on a Mac gets as far as
// preparing the rootfs and fails on a missing debugfs, which reads as
// "install e2fsprogs" when no package would make it work.
func localVMMCheck(id, goos string) error {
	if (id != "fc" && id != "ch") || goos == "linux" {
		return nil
	}
	hint := ""
	if goos == "darwin" {
		hint = "; on a Mac, use -p ac (Apple containers)"
	}
	return fmt.Errorf("the %s provider runs microVMs on KVM, so it needs a Linux host (this is %s): run onctl there%s", id, goos, hint)
}
