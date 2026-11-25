package cmd

import (
	"regexp"
	"testing"

	"github.com/cdalar/onctl/pkg/cloud"
)

// TestTUICreateFormCycle: ←/→ pick from the catalog and wrap, so the
// whole list is reachable with one key; a typed value that isn't in the
// catalog starts from the matching end.
func TestTUICreateFormCycle(t *testing.T) {
	f := newTUICreateForm("", "")
	f.images = []cloud.CloudImage{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	f.field = createFieldImage

	for _, step := range []struct {
		n    int
		want string
	}{{1, "a"}, {1, "b"}, {1, "c"}, {1, "a"}, {-1, "c"}, {-1, "b"}} {
		f.cycle(step.n)
		if got := f.image.Value(); got != step.want {
			t.Fatalf("after cycle(%d) image = %q, want %q", step.n, got, step.want)
		}
	}

	f.image.SetValue("typed-by-hand")
	f.cycle(-1)
	if got := f.image.Value(); got != "c" {
		t.Errorf("left from an unknown value = %q, want the last entry %q", got, "c")
	}
	f.image.SetValue("typed-by-hand")
	f.cycle(1)
	if got := f.image.Value(); got != "a" {
		t.Errorf("right from an unknown value = %q, want the first entry %q", got, "a")
	}
}

// TestTUICreateFormCycleIgnored: with nothing to pick from, or on the
// free-text name field, cycling must leave what was typed alone.
func TestTUICreateFormCycleIgnored(t *testing.T) {
	f := newTUICreateForm("mine", "")
	f.field = createFieldImage
	f.cycle(1)
	if got := f.image.Value(); got != "mine" {
		t.Errorf("empty catalog changed image to %q", got)
	}

	f.images = []cloud.CloudImage{{Name: "a"}}
	f.field = createFieldName
	f.name.SetValue("vm1")
	f.cycle(1)
	if got := f.name.Value(); got != "vm1" {
		t.Errorf("name field changed to %q", got)
	}
}

func TestTUICreateFormCycleType(t *testing.T) {
	f := newTUICreateForm("", "")
	f.sizes = []cloud.CloudSize{{Name: "small"}, {Name: "big"}}
	f.field = createFieldType
	f.cycle(1)
	f.cycle(1)
	if got := f.typ.Value(); got != "big" {
		t.Errorf("type = %q, want big", got)
	}
}

// TestTUICreateFormHints: the hint beside a field tells the user what
// they picked, and falls back to what's on offer or that it's free text.
func TestTUICreateFormHints(t *testing.T) {
	f := newTUICreateForm("img", "big")
	if f.imageHint() != "listing…" || f.typeHint() != "listing…" {
		t.Errorf("before the catalog loads both hints should say listing…, got %q / %q", f.imageHint(), f.typeHint())
	}

	f.loaded = true
	if f.imageHint() != "typed as is" || f.typeHint() != "typed as is" {
		t.Errorf("empty catalog: got %q / %q", f.imageHint(), f.typeHint())
	}

	f.images = []cloud.CloudImage{{Name: "img", Description: "Ubuntu 24.04"}, {Name: "other"}}
	if got := f.imageHint(); got != "Ubuntu 24.04" {
		t.Errorf("imageHint = %q, want the description", got)
	}
	f.image.SetValue("other") // no description: say what's on offer
	if got := f.imageHint(); got != "2 images listed" {
		t.Errorf("imageHint = %q, want the count", got)
	}

	f.sizes = []cloud.CloudSize{{Name: "big", VCPU: 4, MemoryMiB: 8192, DiskGiB: 80, HourlyPrice: "€0.02", Default: true}}
	if got, want := f.typeHint(), "4 vCPU, 8 GiB, 80 GiB disk, €0.02/h, (default)"; got != want {
		t.Errorf("typeHint = %q, want %q", got, want)
	}
	f.typ.SetValue("custom")
	if got := f.typeHint(); got != "1 sizes listed" {
		t.Errorf("typeHint = %q, want the count", got)
	}
}

// TestFlashTenses: the flash after an action names what happened, and
// verbs ending in e must not come out as "pauseed" or "pauseing".
func TestFlashTenses(t *testing.T) {
	for verb, want := range map[string][2]string{
		"pause":   {"pausing", "paused"},
		"resume":  {"resuming", "resumed"},
		"create":  {"creating", "created"},
		"destroy": {"destroying", "destroyed"},
		"restart": {"restarting", "restarted"},
	} {
		if got := presentTense(verb); got != want[0] {
			t.Errorf("presentTense(%q) = %q, want %q", verb, got, want[0])
		}
		if got := pastTense(verb); got != want[1] {
			t.Errorf("pastTense(%q) = %q, want %q", verb, got, want[1])
		}
	}
}

// TestRepoBranch: every clone gets a branch of its own, so two runs on
// one box can't collide.
func TestRepoBranch(t *testing.T) {
	re := regexp.MustCompile(`^onctl/[0-9a-f]{6}$`)
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		b := repoBranch()
		if !re.MatchString(b) {
			t.Fatalf("repoBranch() = %q, want onctl/<6 hex>", b)
		}
		seen[b] = true
	}
	if len(seen) < 2 {
		t.Errorf("repoBranch returned the same branch %d times", 20)
	}
}
