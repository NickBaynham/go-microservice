package config

import (
	"reflect"
	"testing"
)

func TestCORSAllowedOriginsFromEnv(t *testing.T) {
	t.Parallel()
	defaultDev := []string{
		"http://localhost:5173",
		"http://127.0.0.1:5173",
		"http://localhost:4173",
		"http://127.0.0.1:4173",
	}
	tests := []struct {
		name string
		raw  string
		env  string
		want []string
	}{
		{name: "empty development uses defaults", raw: "", env: "development", want: defaultDev},
		{name: "empty test uses defaults", raw: "", env: "test", want: defaultDev},
		{name: "empty production nil", raw: "", env: "production", want: nil},
		{name: "empty prod nil", raw: "", env: "prod", want: nil},
		{name: "comma separated", raw: "https://a.com, https://b.com:3000 ", env: "production", want: []string{"https://a.com", "https://b.com:3000"}},
		{name: "explicit overrides dev", raw: "https://only.one", env: "development", want: []string{"https://only.one"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CORSAllowedOriginsFromEnv(tt.raw, tt.env)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("CORSAllowedOriginsFromEnv(%q, %q) = %#v, want %#v", tt.raw, tt.env, got, tt.want)
			}
		})
	}
}
