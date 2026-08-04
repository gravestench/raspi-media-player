package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	params := DefaultPasswordParams()
	params.Memory = 1024
	params.Iterations = 1
	hash, err := HashPassword("correct horse battery staple", params)
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password was not hashed")
	}
	valid, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !valid {
		t.Fatalf("valid password rejected: valid=%v err=%v", valid, err)
	}
	valid, err = VerifyPassword("incorrect password", hash)
	if err != nil || valid {
		t.Fatalf("invalid password accepted: valid=%v err=%v", valid, err)
	}
	hash2, err := HashPassword("correct horse battery staple", params)
	if err != nil {
		t.Fatal(err)
	}
	if hash == hash2 {
		t.Fatal("password hashes did not use unique salts")
	}
}

func TestUsernameValidation(t *testing.T) {
	display, key, err := NormalizeUsername("  House_DJ  ")
	if err != nil || display != "House_DJ" || key != "house_dj" {
		t.Fatalf("unexpected normalization: %q %q %v", display, key, err)
	}
	for _, invalid := range []string{"a", "bad name", "email@example.com", strings.Repeat("x", 33)} {
		if _, _, err := NormalizeUsername(invalid); err == nil {
			t.Fatalf("accepted invalid username %q", invalid)
		}
	}
}
