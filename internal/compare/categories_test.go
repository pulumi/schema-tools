package compare

import "testing"

func TestKindSeverityMarksMissingFieldsDanger(t *testing.T) {
	for _, kind := range []ChangeKind{
		ChangeKindMissingInput,
		ChangeKindMissingOutput,
		ChangeKindMissingProperty,
	} {
		t.Run(string(kind), func(t *testing.T) {
			if got := kindSeverity(kind, "", nil); got != SeverityDanger {
				t.Fatalf("kindSeverity(%q) = %q, want %q", kind, got, SeverityDanger)
			}
		})
	}
}

func TestRequiredToOptionalClassification(t *testing.T) {
	if !kindBreaking(ChangeKindRequiredToOptional) {
		t.Fatal("required-to-optional changes must be breaking")
	}

	tests := []struct {
		name     string
		category string
		path     []string
		want     Severity
	}{
		{
			name:     "resource output",
			category: resourcesCategory,
			path:     []string{"required", "value"},
			want:     SeverityDanger,
		},
		{
			name:     "function output",
			category: functionsCategory,
			path:     []string{"outputs", "required", "value"},
			want:     SeverityDanger,
		},
		{
			name:     "shared type",
			category: typesCategory,
			path:     []string{"required", "value"},
			want:     SeverityWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kindSeverity(ChangeKindRequiredToOptional, tt.category, tt.path); got != tt.want {
				t.Fatalf("kindSeverity() = %q, want %q", got, tt.want)
			}
		})
	}
}
