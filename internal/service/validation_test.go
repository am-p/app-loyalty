package service

import "testing"

func TestValidPasswordHonorsBcryptByteLimit(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "minimum", value: "0123456789", valid: true},
		{name: "maximum bytes", value: string(make([]byte, 72)), valid: true},
		{name: "too long for bcrypt", value: string(make([]byte, 73)), valid: false},
		{name: "unicode byte overflow", value: "contraseña-segura-para-pruebas-🔐🔐🔐🔐🔐🔐🔐🔐🔐🔐🔐🔐", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validPassword(tt.value); got != tt.valid {
				t.Fatalf("validPassword byte_length=%d = %v, want %v", len(tt.value), got, tt.valid)
			}
		})
	}
}
