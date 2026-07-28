package str

import "testing"

func TestGenerateRandomPassword(t *testing.T) {
	for range 32 {
		password, err := GenerateRandomPassword()
		if err != nil {
			t.Fatalf("generate password: %v", err)
		}
		if len(password) != passwordLength {
			t.Fatalf("expected password length %d, got %d", passwordLength, len(password))
		}
		for _, char := range password {
			valid := false
			for _, allowed := range charset {
				if char == allowed {
					valid = true
					break
				}
			}
			if !valid {
				t.Fatalf("generated password contains invalid character %q", char)
			}
		}
	}
}
