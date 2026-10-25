package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

// A generated box name, for a box nobody named: <prefix>-<slug>-<id>.
// The prefix is the project directory's name, the slug two or three
// words of the prompt -- or, without one, an adjective-noun pair like
// boxctl's dashboard suggests (boxctl-web's lib/nameGenerator.ts; keep the
// word lists in step, boxctl's cmd/boxname.go has the same) -- and the id
// four hex characters. The branch the box works on is onctl/<slug>-<id>,
// so the two can be matched by eye.
type boxName struct{ prefix, slug, id string }

const (
	boxNameMax = 40 // the whole name
	slugMax    = 20 // the slug alone
)

var (
	nameUnsafe     = regexp.MustCompile(`[^a-z0-9]+`)
	nameAdjectives = []string{
		"brave", "calm", "clever", "cosmic", "eager", "fuzzy", "gentle", "happy",
		"jolly", "keen", "lively", "mighty", "nimble", "plucky", "quiet", "rapid",
		"silly", "snappy", "sunny", "swift", "tidy", "witty", "zesty", "bold",
		"bright", "cheerful", "cozy", "crisp", "curious", "daring",
	}
	nameNouns = []string{
		"otter", "falcon", "comet", "maple", "harbor", "canyon", "meadow",
		"rocket", "beetle", "pepper", "willow", "tiger", "puffin", "glacier",
		"lantern", "orchid", "badger", "cobra", "dune", "ember", "fjord",
		"heron", "island", "jaguar", "kite", "lynx", "nebula", "otterhound",
		"pelican", "quokka",
	}
	// Words that say little about a prompt, left out of its slug.
	slugStopWords = map[string]bool{
		"a": true, "an": true, "the": true, "to": true, "and": true, "or": true,
		"of": true, "for": true, "in": true, "on": true, "at": true, "by": true,
		"with": true, "it": true, "its": true, "that": true, "this": true,
		"what": true, "is": true, "be": true, "so": true, "please": true,
	}
)

// box is the box's name: the prefix cut short as needed to keep the
// whole within boxNameMax.
func (n boxName) box() string {
	rest := n.slug + "-" + n.id
	prefix := n.prefix
	if room := boxNameMax - len(rest) - 1; len(prefix) > room {
		prefix = strings.Trim(prefix[:max(room, 0)], "-")
	}
	if prefix == "" {
		return rest
	}
	return prefix + "-" + rest
}

// branch is the branch the box works on.
func (n boxName) branch() string { return "onctl/" + n.slug + "-" + n.id }

// nameSlug is a name part: lowercase letters, digits and single dashes.
func nameSlug(s string) string {
	return strings.Trim(nameUnsafe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// textSlug is two or three words of text, less the ones that say little,
// within slugMax characters -- or "" when it has none.
func textSlug(text string) string {
	var words []string
	for _, w := range strings.Split(nameSlug(text), "-") {
		if w == "" || slugStopWords[w] {
			continue
		}
		if len(words) > 0 && len(strings.Join(append(words, w), "-")) > slugMax {
			break
		}
		words = append(words, w)
		if len(words) == 3 {
			break
		}
	}
	slug := strings.Join(words, "-")
	if len(slug) > slugMax { // one long word
		slug = strings.Trim(slug[:slugMax], "-")
	}
	return slug
}

func randomIndex(n int) int {
	i, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err) // crypto/rand doesn't fail
	}
	return int(i.Int64())
}

func newNameID() string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// generateName is a name for prefix's box doing text (which may be ""),
// not one of taken: a new id for as long as it is.
func generateName(prefix, text string, taken func(string) bool) (boxName, error) {
	n := boxName{prefix: nameSlug(prefix), slug: textSlug(text)}
	if n.slug == "" {
		n.slug = nameAdjectives[randomIndex(len(nameAdjectives))] + "-" + nameNouns[randomIndex(len(nameNouns))]
	}
	for range 20 {
		n.id = newNameID()
		if !taken(n.box()) {
			return n, nil
		}
	}
	return boxName{}, fmt.Errorf("couldn't find a free box name like %s", n.box())
}

// newBoxName is generateName against the VMs the provider has.
func newBoxName(ctx context.Context, prefix, text string) (boxName, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	vms, err := listClaudeVMs(ctx)
	if err != nil {
		return boxName{}, fmt.Errorf("listing your VMs for a free name: %w", err)
	}
	names := make(map[string]bool, len(vms))
	for _, vm := range vms {
		names[vm.name] = true
	}
	return generateName(prefix, text, func(s string) bool { return names[s] })
}
