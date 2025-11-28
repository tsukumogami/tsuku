package version

import "testing"

func TestIsValidPyPIPackageName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"requests", true},
		{"flask", true},
		{"django-rest-framework", true},
		{"python_dateutil", true},
		{"boto3", true},
		{"package.name", true},

		// Invalid names
		{"", false},
		{"../etc/passwd", false},
		{"/etc/passwd", false},
		{"\\windows\\system32", false},
		{"-starts-with-dash", false},
		{"has spaces", false},
		{"has@special", false},
		{"UPPERCASE", false},
		{string(make([]byte, 215)), false}, // Too long
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidPyPIPackageName(tt.name)
			if result != tt.expected {
				t.Errorf("isValidPyPIPackageName(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}
