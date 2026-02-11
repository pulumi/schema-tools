package compare

import (
	"reflect"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/schema-tools/internal/util/set"
)

func TestPluralizationCandidates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "singular", in: "filter", want: []string{"filters"}},
		{name: "plural", in: "filters", want: []string{"filter"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pluralizationCandidates(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("pluralizationCandidates(%q): got %v want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestMaxItemsOneRename(t *testing.T) {
	t.Parallel()

	oldProp := scalarProperty("string")
	tests := []struct {
		name     string
		oldName  string
		newProps map[string]schema.PropertySpec
		wantName string
		wantOK   bool
	}{
		{
			name:    "detects singular to plural max-items-one transition",
			oldName: "filter",
			newProps: map[string]schema.PropertySpec{
				"filters": arrayProperty("string"),
			},
			wantName: "filters",
			wantOK:   true,
		},
		{
			name:    "rejects candidate when element type changed",
			oldName: "filter",
			newProps: map[string]schema.PropertySpec{
				"filters": arrayProperty("integer"),
			},
			wantOK: false,
		},
		{
			name:    "rejects when candidate key missing",
			oldName: "filter",
			newProps: map[string]schema.PropertySpec{
				"other": arrayProperty("string"),
			},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotName, gotOK := maxItemsOneRename(tc.oldName, oldProp, tc.newProps)
			if gotOK != tc.wantOK || gotName != tc.wantName {
				t.Fatalf("maxItemsOneRename(%q): got (%q,%v) want (%q,%v)", tc.oldName, gotName, gotOK, tc.wantName, tc.wantOK)
			}
		})
	}
}

func TestIsTrueRename(t *testing.T) {
	t.Parallel()

	baseOld := map[string]schema.PropertySpec{
		"filter": scalarProperty("string"),
	}
	baseNew := map[string]schema.PropertySpec{
		"filters": arrayProperty("string"),
	}

	tests := []struct {
		name     string
		oldName  string
		newName  string
		oldProps map[string]schema.PropertySpec
		newProps map[string]schema.PropertySpec
		want     bool
	}{
		{
			name:     "true rename when old removed and new introduced",
			oldName:  "filter",
			newName:  "filters",
			oldProps: baseOld,
			newProps: baseNew,
			want:     true,
		},
		{
			name:     "false when old key still exists in new props",
			oldName:  "filter",
			newName:  "filters",
			oldProps: baseOld,
			newProps: map[string]schema.PropertySpec{"filter": scalarProperty("string"), "filters": arrayProperty("string")},
			want:     false,
		},
		{
			name:     "false when new key already existed in old props",
			oldName:  "filter",
			newName:  "filters",
			oldProps: map[string]schema.PropertySpec{"filter": scalarProperty("string"), "filters": arrayProperty("string")},
			newProps: baseNew,
			want:     false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isTrueRename(tc.oldName, tc.newName, tc.oldProps, tc.newProps)
			if got != tc.want {
				t.Fatalf("isTrueRename(%q,%q): got %v want %v", tc.oldName, tc.newName, got, tc.want)
			}
		})
	}
}

func TestIsMaxItemsOneRenameRequired(t *testing.T) {
	t.Parallel()

	oldRequired := set.FromSlice([]string{"filter"})
	oldProps := map[string]schema.PropertySpec{
		"filter": scalarProperty("string"),
	}

	tests := []struct {
		name     string
		newName  string
		newProps map[string]schema.PropertySpec
		want     bool
	}{
		{
			name:    "suppresses optional-to-required on true rename",
			newName: "filters",
			newProps: map[string]schema.PropertySpec{
				"filters": arrayProperty("string"),
			},
			want: true,
		},
		{
			name:    "does not suppress when singular and plural coexist",
			newName: "filters",
			newProps: map[string]schema.PropertySpec{
				"filter":  scalarProperty("string"),
				"filters": arrayProperty("string"),
			},
			want: false,
		},
		{
			name:    "does not suppress when element type changes",
			newName: "filters",
			newProps: map[string]schema.PropertySpec{
				"filters": arrayProperty("integer"),
			},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isMaxItemsOneRenameRequired(tc.newName, oldRequired, oldProps, tc.newProps)
			if got != tc.want {
				t.Fatalf("isMaxItemsOneRenameRequired(%q): got %v want %v", tc.newName, got, tc.want)
			}
		})
	}
}

func TestIsMaxItemsOneRenameRequiredToOptional(t *testing.T) {
	t.Parallel()

	oldProps := map[string]schema.PropertySpec{
		"filter": scalarProperty("string"),
	}

	tests := []struct {
		name        string
		oldName     string
		newRequired set.Set[string]
		newProps    map[string]schema.PropertySpec
		want        bool
	}{
		{
			name:        "suppresses required-to-optional on true rename",
			oldName:     "filter",
			newRequired: set.FromSlice([]string{"filters"}),
			newProps: map[string]schema.PropertySpec{
				"filters": arrayProperty("string"),
			},
			want: true,
		},
		{
			name:        "does not suppress when singular and plural coexist",
			oldName:     "filter",
			newRequired: set.FromSlice([]string{"filters"}),
			newProps: map[string]schema.PropertySpec{
				"filter":  scalarProperty("string"),
				"filters": arrayProperty("string"),
			},
			want: false,
		},
		{
			name:        "does not suppress when element type changes",
			oldName:     "filter",
			newRequired: set.FromSlice([]string{"filters"}),
			newProps: map[string]schema.PropertySpec{
				"filters": arrayProperty("integer"),
			},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isMaxItemsOneRenameRequiredToOptional(tc.oldName, tc.newRequired, oldProps, tc.newProps)
			if got != tc.want {
				t.Fatalf("isMaxItemsOneRenameRequiredToOptional(%q): got %v want %v", tc.oldName, got, tc.want)
			}
		})
	}
}

func BenchmarkPluralizationCandidates(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = pluralizationCandidates("filter")
	}
}

func BenchmarkMaxItemsOneRenameRequiredToOptional(b *testing.B) {
	b.ReportAllocs()
	oldProps := map[string]schema.PropertySpec{
		"filter": scalarProperty("string"),
	}
	newRequired := set.FromSlice([]string{"filters"})
	newProps := map[string]schema.PropertySpec{
		"filters": arrayProperty("string"),
	}
	for i := 0; i < b.N; i++ {
		_ = isMaxItemsOneRenameRequiredToOptional("filter", newRequired, oldProps, newProps)
	}
}

func scalarProperty(typ string) schema.PropertySpec {
	return schema.PropertySpec{
		TypeSpec: schema.TypeSpec{Type: typ},
	}
}

func arrayProperty(itemType string) schema.PropertySpec {
	return schema.PropertySpec{
		TypeSpec: schema.TypeSpec{
			Type: "array",
			Items: &schema.TypeSpec{
				Type: itemType,
			},
		},
	}
}
