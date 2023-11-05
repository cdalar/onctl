package rand

import (
	"math/rand"
	"time"
)

// const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
const charset = "abcdefghjkmnpqrstuvwxyz23456789" //Some chars i,l,o,1,0 removed
const charalphabetic = "abcdefghjkmnpqrstuvwxyz"  //Only Alphabetic characters and Some chars i,l,o, removed
const passwordSet = "abcdefghjkmnpqrstuvwxyz23456789!@#$%^&*()_+"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

// StringWithCharset .. returns random string with charset
func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	b[0] = charset[seededRand.Intn(len(charalphabetic))]
	for i := 1; i < length; i++ {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// String return random string with length
func String(length int) string {
	return StringWithCharset(length, charset)
}

func Password(length int) string {
	return StringWithCharset(length, passwordSet)
}
