package middleware

import "testing"

func TestValidateJWTConfig(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		secret  string
		wantErr bool
	}{
		{name: "development permits ephemeral secret", env: "development", wantErr: false},
		{name: "test permits ephemeral secret", env: "test", wantErr: false},
		{name: "production requires secret", env: "production", wantErr: true},
		{name: "unset environment fails safe", env: "", wantErr: true},
		{name: "mixed case production fails safe", env: " Production ", wantErr: true},
		{name: "configured secret is accepted", env: "production", secret: "secret-for-test", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ENV", tt.env)
			t.Setenv("JWT_SECRET", tt.secret)
			if err := ValidateJWTConfig(); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateJWTConfig() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
