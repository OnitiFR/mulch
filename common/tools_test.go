package common

import (
	"testing"
	"time"
)

func TestStringIsVariable(t *testing.T) {
	tests := []struct {
		name      string
		s         string
		varName   string
		wantOk    bool
		wantValue string
	}{
		{
			name:      "matching variable",
			s:         "FOOBAR=dummy",
			varName:   "FOOBAR",
			wantOk:    true,
			wantValue: "dummy",
		},
		{
			name:      "no prefix match",
			s:         "NOTFOO=dummy",
			varName:   "FOO",
			wantOk:    false,
			wantValue: "",
		},
		{
			name:      "empty value",
			s:         "FOO=",
			varName:   "FOO",
			wantOk:    true,
			wantValue: "",
		},
		{
			name:      "value with equals sign",
			s:         "FOO=bar=baz",
			varName:   "FOO",
			wantOk:    true,
			wantValue: "bar=baz",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, val := StringIsVariable(tc.s, tc.varName)
			if ok != tc.wantOk {
				t.Errorf("StringIsVariable(%q, %q) ok = %v, want %v", tc.s, tc.varName, ok, tc.wantOk)
			}
			if val != tc.wantValue {
				t.Errorf("StringIsVariable(%q, %q) value = %q, want %q", tc.s, tc.varName, val, tc.wantValue)
			}
		})
	}
}

func TestStringFindVariables(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantCount    int
		wantContains []string
	}{
		{
			name:      "no variables",
			input:     "plain text without vars",
			wantCount: 0,
		},
		{
			name:         "single variable at end",
			input:        "value is $foo",
			wantCount:    1,
			wantContains: []string{"foo"},
		},
		{
			name:         "multiple variables",
			input:        "$foo and $bar plus $foo again",
			wantCount:    2,
			wantContains: []string{"foo", "bar"},
		},
		{
			name:         "variable followed by separator",
			input:        "$foo,$bar",
			wantCount:    2,
			wantContains: []string{"foo", "bar"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StringFindVariables(tc.input)
			if len(got) != tc.wantCount {
				t.Fatalf("StringFindVariables(%q) returned %d vars, want %d (got %v)", tc.input, len(got), tc.wantCount, got)
			}
			for _, want := range tc.wantContains {
				found := false
				for _, g := range got {
					if g == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("StringFindVariables(%q) missing expected variable %q (got %v)", tc.input, want, got)
				}
			}
		})
	}
}

func TestStringExpandVariables(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		variables map[string]interface{}
		want      string
	}{
		{
			name:      "no variables in string",
			input:     "plain text",
			variables: map[string]interface{}{"foo": "bar"},
			want:      "plain text",
		},
		{
			name:      "single string variable expanded",
			input:     "hello $name",
			variables: map[string]interface{}{"name": "world"},
			want:      "hello world",
		},
		{
			name:      "integer variable expanded",
			input:     "count=$n",
			variables: map[string]interface{}{"n": 42},
			want:      "count=42",
		},
		{
			name:      "variable not in map left as is",
			input:     "value is $unknown",
			variables: map[string]interface{}{"foo": "bar"},
			want:      "value is $unknown",
		},
		{
			name:      "multiple variables expanded",
			input:     "$greeting $subject",
			variables: map[string]interface{}{"greeting": "hello", "subject": "world"},
			want:      "hello world",
		},
		{
			name:      "bool variable expanded",
			input:     "enabled=$flag",
			variables: map[string]interface{}{"flag": true},
			want:      "enabled=true",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StringExpandVariables(tc.input, tc.variables)
			if got != tc.want {
				t.Errorf("StringExpandVariables(%q, %v) = %q, want %q", tc.input, tc.variables, got, tc.want)
			}
		})
	}
}

func TestInterfaceValueToString(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want string
	}{
		{"int", 42, "42"},
		{"int64", int64(9223372036854775807), "9223372036854775807"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float64", 3.14, "3.14"},
		{"string", "hello", "hello"},
		{"[]byte", []byte("bytes"), "bytes"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"[]string", []string{"a", "b", "c"}, "a, b, c"},
		{"time.Duration", 5 * time.Second, "5s"},
		{"unsupported type", struct{}{}, "INVALID_TYPE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := InterfaceValueToString(tc.in)
			if got != tc.want {
				t.Errorf("InterfaceValueToString(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
