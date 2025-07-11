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
	const expectedString10Chars = "hsgqfg8vc2"
	if str != expectedString10Chars {
		t.Errorf("got %s, want %s", str, expectedString10Chars)
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
