package terminal

import (
	"strings"
	"testing"
)

func TestPrintableANSIOutputDropsEscapeSequences(t *testing.T) {
	input := "diff --git a/main.go b/main.go\n\x1b[31m-old\x1b[0m\n\x1b]8;;https://example.com\x1b\\+new\x1b]8;;\x1b\\\n"
	if got, want := PrintableANSIOutput(strings.NewReader(input)), "diff --git a/main.go b/main.go\n-old\n+new\n"; got != want {
		t.Fatalf("PrintableANSIOutput() = %q, want %q", got, want)
	}
}

func TestPrintableANSIOutputPreservesTabs(t *testing.T) {
	input := "+\tindented\n"
	if got, want := PrintableANSIOutput(strings.NewReader(input)), input; got != want {
		t.Fatalf("PrintableANSIOutput() = %q, want %q", got, want)
	}
}
