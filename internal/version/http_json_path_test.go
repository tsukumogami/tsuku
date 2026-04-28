package version

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseVersionPath_Valid(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []pathStep
	}{
		{
			name: "single key",
			path: "version",
			want: []pathStep{{key: "version"}},
		},
		{
			name: "dotted nested",
			path: "current.version",
			want: []pathStep{{key: "current"}, {key: "version"}},
		},
		{
			name: "key then index",
			path: "releases[0]",
			want: []pathStep{{key: "releases"}, {isIndex: true, index: 0}},
		},
		{
			name: "key then index then key",
			path: "releases[0].version",
			want: []pathStep{
				{key: "releases"},
				{isIndex: true, index: 0},
				{key: "version"},
			},
		},
		{
			name: "leading index",
			path: "[0]",
			want: []pathStep{{isIndex: true, index: 0}},
		},
		{
			name: "leading index then dotted key chain",
			path: "[0].version.openjdk_version",
			want: []pathStep{
				{isIndex: true, index: 0},
				{key: "version"},
				{key: "openjdk_version"},
			},
		},
		{
			name: "key with underscore",
			path: "current_version",
			want: []pathStep{{key: "current_version"}},
		},
		{
			name: "key with hyphen",
			path: "release-version",
			want: []pathStep{{key: "release-version"}},
		},
		{
			name: "double-digit index",
			path: "items[42].name",
			want: []pathStep{
				{key: "items"},
				{isIndex: true, index: 42},
				{key: "name"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVersionPath(tt.path)
			if err != nil {
				t.Fatalf("parseVersionPath(%q) failed: %v", tt.path, err)
			}
			if !equalSteps(got, tt.want) {
				t.Errorf("parseVersionPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseVersionPath_Invalid(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantError string // substring expected in the error message
	}{
		{"empty", "", "path is empty"},
		{"leading dot", ".version", "path cannot start with `.`"},
		{"trailing dot", "version.", "unexpected `.`"},
		{"double dot", "current..version", "unexpected `.`"},
		{"unterminated bracket", "releases[0", "unterminated `[`"},
		{"empty bracket", "releases[]", "empty `[]`"},
		{"non-numeric index", "releases[abc]", "invalid array index"},
		{"negative index", "releases[-1]", "negative array index"},
		{"missing separator", "releases[0]version", "expected `.` or `[`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseVersionPath(tt.path)
			if err == nil {
				t.Fatalf("parseVersionPath(%q) succeeded; want error containing %q", tt.path, tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("parseVersionPath(%q) error = %q, want substring %q", tt.path, err.Error(), tt.wantError)
			}
		})
	}
}

func TestWalkPath_Success(t *testing.T) {
	tests := []struct {
		name string
		json string
		path string
		want any
	}{
		{
			name: "top-level field",
			json: `{"version": "566.0.0"}`,
			path: "version",
			want: "566.0.0",
		},
		{
			name: "nested field",
			json: `{"current": {"version": "1.20.4"}}`,
			path: "current.version",
			want: "1.20.4",
		},
		{
			name: "array index then field",
			json: `{"releases": [{"version": "21.0.4_7"}]}`,
			path: "releases[0].version",
			want: "21.0.4_7",
		},
		{
			name: "leading array index",
			json: `[{"version": {"openjdk_version": "21.0.4+7"}}]`,
			path: "[0].version.openjdk_version",
			want: "21.0.4+7",
		},
		{
			name: "numeric leaf",
			json: `{"most_recent_lts": 21}`,
			path: "most_recent_lts",
			want: float64(21),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := parseVersionPath(tt.path)
			if err != nil {
				t.Fatalf("parseVersionPath(%q) failed: %v", tt.path, err)
			}
			var root any
			if err := json.Unmarshal([]byte(tt.json), &root); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			got, err := walkPath(root, steps)
			if err != nil {
				t.Fatalf("walkPath failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("walkPath = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestWalkPath_Errors(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		path      string
		wantError string
	}{
		{
			name:      "key missing",
			json:      `{"foo": "bar"}`,
			path:      "missing",
			wantError: "key not found",
		},
		{
			name:      "expected object got array",
			json:      `["a", "b"]`,
			path:      "version",
			wantError: "expected object, got array",
		},
		{
			name:      "expected array got object",
			json:      `{"version": "1.0"}`,
			path:      "[0]",
			wantError: "expected array, got object",
		},
		{
			name:      "index out of range",
			json:      `{"items": []}`,
			path:      "items[0]",
			wantError: "index out of range",
		},
		{
			name:      "type mismatch midway",
			json:      `{"current": "not-an-object"}`,
			path:      "current.version",
			wantError: "expected object, got string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := parseVersionPath(tt.path)
			if err != nil {
				t.Fatalf("parseVersionPath failed: %v", err)
			}
			var root any
			if err := json.Unmarshal([]byte(tt.json), &root); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			_, err = walkPath(root, steps)
			if err == nil {
				t.Fatalf("walkPath succeeded; want error containing %q", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("walkPath error = %q, want substring %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestStringifyLeaf(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		want      string
		wantError bool
	}{
		{"string", "566.0.0", "566.0.0", false},
		{"integer-valued float", float64(21), "21", false},
		{"non-integer float", float64(1.5), "1.5", false},
		{"bool rejected", true, "", true},
		{"null rejected", nil, "", true},
		{"object rejected", map[string]any{"a": 1}, "", true},
		{"array rejected", []any{1, 2}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stringifyLeaf(tt.input)
			if tt.wantError {
				if err == nil {
					t.Errorf("stringifyLeaf(%v) succeeded; want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("stringifyLeaf(%v) failed: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("stringifyLeaf(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func equalSteps(a, b []pathStep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
