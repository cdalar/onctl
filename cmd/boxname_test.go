package cmd

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestTextSlug: the slug is what someone would recognise the prompt by --
// a few meaningful words, never a stop word, never over the limit.
func TestTextSlug(t *testing.T) {
	for text, want := range map[string]string{
		"Fix the flaky auth test!":                       "fix-flaky-auth",
		"bump Go to 1.24 and fix everything that breaks": "bump-go-1",
		"  ***  ":                            "",
		"the a an":                           "",
		"İstanbul'a taşı":                    "istanbul-ta",
		"internationalization of everything": "internationalization",
		"supercalifragilisticexpialidocious words": "supercalifragilistic",
		"refactor authentication middleware now":   "refactor",
	} {
		if got := textSlug(text); got != want {
			t.Errorf("textSlug(%q) = %q, want %q", text, got, want)
		}
	}
}

var generatedName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// TestGenerateName: a box's name and its branch share the slug and id, so
// the two can be matched by eye; the name fits every provider's limits;
// and a taken name is never reused.
func TestGenerateName(t *testing.T) {
	none := func(string) bool { return false }

	n, err := generateName("onctl", "Fix the flaky auth test", none)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(n.box(), "onctl-fix-flaky-auth-") || len(n.id) != 4 {
		t.Errorf("box %q", n.box())
	}
	if n.branch() != "onctl/fix-flaky-auth-"+n.id || !strings.HasSuffix(n.box(), strings.TrimPrefix(n.branch(), "onctl/")) {
		t.Errorf("branch %q doesn't match box %q", n.branch(), n.box())
	}

	// No text: an adjective-noun pair.
	n, _ = generateName("My_Project", "", none)
	adj, noun, _ := strings.Cut(n.slug, "-")
	if !slices.Contains(nameAdjectives, adj) || !slices.Contains(nameNouns, noun) || !strings.HasPrefix(n.box(), "my-project-") {
		t.Errorf("box %q", n.box())
	}

	// No prefix: just the slug and id.
	n, _ = generateName("", "deploy it", none)
	if n.box() != "deploy-"+n.id {
		t.Errorf("box %q", n.box())
	}

	// Long names are cut to 40 from the prefix, never the slug or id.
	n, _ = generateName(strings.Repeat("verylongrepositoryname-", 3), "Fix the flaky auth test", none)
	if b := n.box(); len(b) > boxNameMax || !strings.HasSuffix(b, "-fix-flaky-auth-"+n.id) || !generatedName.MatchString(b) {
		t.Errorf("box %q (%d)", b, len(b))
	}
	for range 50 {
		n, _ = generateName("a-repository-with-a-long-name", "", none)
		if b := n.box(); len(b) > boxNameMax || !generatedName.MatchString(b) {
			t.Errorf("box %q (%d)", b, len(b))
		}
	}

	// A taken name gets a new id.
	first := ""
	n, err = generateName("r", "x", func(s string) bool {
		if first == "" {
			first = s
			return true
		}
		return s == first
	})
	if err != nil || n.box() == first {
		t.Errorf("got %q, %v after %q was taken", n.box(), err, first)
	}
	if _, err := generateName("r", "x", func(string) bool { return true }); err == nil {
		t.Error("no error with every name taken")
	}
}
