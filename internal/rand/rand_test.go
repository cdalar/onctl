package rand

import (
	"strings"
	"testing"
)

func TestPassword(t *testing.T) {
	pwd := Password(12)
	if len(pwd) != 12 {
		t.Errorf("Password length should be 12, got %d", len(pwd))
	}
	// Password should start with an alphabetic character
	const numericset = "0123456789"
	if strings.Contains(numericset, string(pwd[0])) {
		t.Errorf("Password should not start with numeric char, got: %s", pwd)
	}
}

func TestPassword_Length(t *testing.T) {
	for _, l := range []int{8, 16, 32} {
		pwd := Password(l)
		if len(pwd) != l {
			t.Errorf("Password(%d) length = %d, want %d", l, len(pwd), l)
		}
	}
}

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
