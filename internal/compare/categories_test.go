package compare

import "testing"

func TestKindSeverityMarksMissingFieldsDanger(t *testing.T) {
	for _, kind := range []ChangeKind{
		ChangeKindMissingInput,
		ChangeKindMissingOutput,
		ChangeKindMissingProperty,
	} {
		t.Run(string(kind), func(t *testing.T) {
			if got := kindSeverity(kind, nil); got != SeverityDanger {
				t.Fatalf("kindSeverity(%q) = %q, want %q", kind, got, SeverityDanger)
			}
		})
	}
}
