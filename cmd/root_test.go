package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestNormalizeOutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "json", input: "json", want: "json"},
		{name: "id", input: "id", want: "id"},
		{name: "table", input: "table", want: "table"},
		{name: "default alias", input: "default", want: "table"},
		{name: "case insensitive", input: "JSON", want: "json"},
		{name: "invalid", input: "yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeOutputFormat(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeOutputFormat(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveOutputFormatExplicitFormat(t *testing.T) {
	var outputValue string
	c := &cobra.Command{Use: "test"}
	c.Flags().StringVar(&outputValue, "format", "table", "")
	c.Flags().StringVar(&outputValue, "output", "table", "")
	if err := c.Flags().Set("format", "id"); err != nil {
		t.Fatalf("failed to set --format: %v", err)
	}

	got, err := ResolveOutputFormat(c)
	if err != nil {
		t.Fatalf("ResolveOutputFormat returned error: %v", err)
	}
	if got != "id" {
		t.Fatalf("ResolveOutputFormat() = %q, want %q", got, "id")
	}
}

func TestResolveOutputFormatLegacyOutputAlias(t *testing.T) {
	var outputValue string
	c := &cobra.Command{Use: "test"}
	c.Flags().StringVar(&outputValue, "format", "table", "")
	c.Flags().StringVar(&outputValue, "output", "table", "")
	if err := c.Flags().Set("output", "default"); err != nil {
		t.Fatalf("failed to set --output: %v", err)
	}

	got, err := ResolveOutputFormat(c)
	if err != nil {
		t.Fatalf("ResolveOutputFormat returned error: %v", err)
	}
	if got != "table" {
		t.Fatalf("ResolveOutputFormat() = %q, want %q", got, "table")
	}
}

func TestResolveOutputFormatAutoJSONWhenNotTTY(t *testing.T) {
	if isStdoutTTY() {
		t.Skip("stdout is a TTY in this environment")
	}

	var outputValue string
	c := &cobra.Command{Use: "test"}
	c.Flags().StringVar(&outputValue, "format", "table", "")
	c.Flags().StringVar(&outputValue, "output", "table", "")

	got, err := ResolveOutputFormat(c)
	if err != nil {
		t.Fatalf("ResolveOutputFormat returned error: %v", err)
	}
	if got != "json" {
		t.Fatalf("ResolveOutputFormat() = %q, want %q", got, "json")
	}
}
