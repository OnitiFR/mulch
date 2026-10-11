package common

import "testing"

func TestCleanURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "already clean",
			input: "http://example.com/api/v1",
			want:  "http://example.com/api/v1",
		},
		{
			name:  "trailing slash removed",
			input: "http://example.com/api/v1/",
			want:  "http://example.com/api/v1",
		},
		{
			name:  "double slashes collapsed",
			input: "http://example.com//api//v1",
			want:  "http://example.com/api/v1",
		},
		{
			name:  "dot segments resolved",
			input: "http://example.com/api/../v1",
			want:  "http://example.com/v1",
		},
		{
			name:  "current dir segments resolved",
			input: "http://example.com/./api/./v1",
			want:  "http://example.com/api/v1",
		},
		{
			name:  "root path stays root",
			input: "http://example.com/",
			want:  "http://example.com/",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CleanURL(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CleanURL(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("CleanURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRemoveAPIKeyFromString(t *testing.T) {
	tests := []struct {
		name string
		in   string
		key  string
		want string
	}{
		{
			name: "empty key returns original",
			in:   "log line with secret123",
			key:  "",
			want: "log line with secret123",
		},
		{
			name: "single occurrence replaced",
			in:   "Authorization: abc123",
			key:  "abc123",
			want: "Authorization: xxx",
		},
		{
			name: "multiple occurrences replaced",
			in:   "key=abc123 token=abc123 id=abc123",
			key:  "abc123",
			want: "key=xxx token=xxx id=xxx",
		},
		{
			name: "key not present returns original",
			in:   "no secrets here",
			key:  "abc123",
			want: "no secrets here",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RemoveAPIKeyFromString(tc.in, tc.key)
			if got != tc.want {
				t.Errorf("RemoveAPIKeyFromString(%q, %q) = %q, want %q", tc.in, tc.key, got, tc.want)
			}
		})
	}
}
