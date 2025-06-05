package rand

import (
	"strings"
	"testing"
)

func Test_String(t *testing.T) {
	seededRand.Seed(1)
	str := String(10)
	if len(str) != 10 {
		t.Errorf("%s is not 10 chars", str)
	}
	if str != "hsgqfg8vc2" {
		t.Errorf("got %s, want hsgqfg8vc2", str)
	}

	str = String(6)
	if len(str) != 6 {
		t.Errorf("%s is not 6 chars", str)
	}

	const numericset = "0123456789"
	str3 := String(7)
	if strings.Contains(numericset, string(str3[0])) {
		t.Errorf("%s is starting numeric char", str3)
	}
}
