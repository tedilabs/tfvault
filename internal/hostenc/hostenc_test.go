package hostenc

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"app.terraform.io", "app.terraform.io", false},
		{"App.Terraform.IO", "app.terraform.io", false},
		{"  spacelift.io ", "spacelift.io", false},
		{"tfe.example.com:8443", "tfe.example.com:8443", false},
		{"my-registry.example.com", "my-registry.example.com", false},
		{"", "", true},
		{"   ", "", true},
		{"../etc/passwd", "", true},
		{"a/../b", "", true},
		{"host/with/slash", "", true},
		{"-leading-dash.com", "", true},
		{".leading-dot.com", "", true},
		{"host name.com", "", true},
		{"host\nname.com", "", true},
		{"$(evil).com", "", true},
	}
	for _, tt := range tests {
		got, err := Normalize(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Normalize(%q) = %q, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Normalize(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEnvSuffix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"app.terraform.io", "app_terraform_io"},
		{"my-registry.example.com", "my__registry_example_com"},
		{"tfe.example.com:8443", "tfe_example_com_8443"},
	}
	for _, tt := range tests {
		if got := EnvSuffix(tt.in); got != tt.want {
			t.Errorf("EnvSuffix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
