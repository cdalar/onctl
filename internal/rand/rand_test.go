package rand

import (
	"strings"
	"testing"
)

func Test_String(t *testing.T) {
	str := String(10)
	if len(str) != 10 {
		t.Errorf("%s is not 10 chars", str)
	}
	str2 := String(10)
	if str == str2 {
		t.Errorf("%s == %s", str, str2)
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
func Test_Password(t *testing.T) {
	password := Password(10)
	if len(password) != 10 {
		t.Errorf("%s is not 10 chars", password)
	}
	password2 := Password(10)
	if password == password2 {
		t.Errorf("%s == %s", password, password2)
	}

	password = Password(6)
	if len(password) != 6 {
		t.Errorf("%s is not 6 chars", password)
	}

	const specialset = "!@#$%^&*()_+"
	password3 := Password(7)
	if !strings.ContainsAny(password3, specialset) {
		t.Errorf("%s does not contain any special characters", password3)
	}
}
