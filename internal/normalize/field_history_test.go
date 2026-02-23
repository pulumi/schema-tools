package normalize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

type maxItemsFixture struct {
	Old      MetadataEnvelope `json:"old"`
	New      MetadataEnvelope `json:"new"`
	Expected struct {
		Resources   map[string]map[string]FieldPathEvidence `json:"resources"`
		Datasources map[string]map[string]FieldPathEvidence `json:"datasources"`
	} `json:"expected"`
}

func TestFlattenFieldHistoryDeterministicPaths(t *testing.T) {
	t.Parallel()

	metadata, err := ParseMetadata(mustReadFieldHistoryFixture(t, filepath.Join("metadata", "parity-vsphere.json")))
	require.NoError(t, err)

	entry := metadata.AutoAliasing.Resources["vsphere_compute_cluster"]
	require.NotNil(t, entry)

	flat := FlattenFieldHistory(entry.Fields)
	require.Equal(t, []string{"host_image", "host_image[*]", "host_image[*].component", "tags"}, collectBoolPaths(flat))
	require.Equal(t, true, mustBool(t, flat["host_image"]))
	require.Nil(t, flat["host_image[*]"])
	require.Equal(t, false, mustBool(t, flat["host_image[*].component"]))
	require.Equal(t, false, mustBool(t, flat["tags"]))
}

func TestMaxItemsOneTransitionClassifier(t *testing.T) {
	t.Parallel()

	trueValue := boolPtr(true)
	falseValue := boolPtr(false)

	tests := []struct {
		name string
		old  *bool
		new  *bool
		want MaxItemsOneTransition
	}{
		{name: "nil nil", old: nil, new: nil, want: MaxItemsOneTransitionUnknown},
		{name: "nil true", old: nil, new: trueValue, want: MaxItemsOneTransitionUnknown},
		{name: "nil false", old: nil, new: falseValue, want: MaxItemsOneTransitionUnknown},
		{name: "true nil", old: trueValue, new: nil, want: MaxItemsOneTransitionUnknown},
		{name: "false nil", old: falseValue, new: nil, want: MaxItemsOneTransitionUnknown},
		{name: "true true", old: trueValue, new: trueValue, want: MaxItemsOneTransitionUnchanged},
		{name: "false false", old: falseValue, new: falseValue, want: MaxItemsOneTransitionUnchanged},
		{name: "true false", old: trueValue, new: falseValue, want: MaxItemsOneTransitionChanged},
		{name: "false true", old: falseValue, new: trueValue, want: MaxItemsOneTransitionChanged},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, ClassifyMaxItemsOneTransition(tt.old, tt.new))
		})
	}
}

func TestFieldHistoryEvidenceFromFixtures(t *testing.T) {
	t.Parallel()

	fixtures := []string{"nested-transitions.json", "coexistence-edge.json"}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			loaded := loadMaxItemsFixture(t, fixture)

			evidence := BuildFieldHistoryEvidence(&loaded.Old, &loaded.New)
			require.Equal(t, loaded.Expected.Resources, evidence.Resources)
			require.Equal(t, loaded.Expected.Datasources, evidence.Datasources)
		})
	}
}

func TestFieldHistoryCoexistenceSafety(t *testing.T) {
	t.Parallel()

	fixture := loadMaxItemsFixture(t, "coexistence-edge.json")
	evidence := BuildFieldHistoryEvidence(&fixture.Old, &fixture.New)

	resourceEvidence := evidence.Resources["pkg_widget"]
	require.NotNil(t, resourceEvidence)

	filter := resourceEvidence["filter"]
	require.Equal(t, MaxItemsOneTransitionUnchanged, filter.Transition)
	require.Equal(t, true, mustBool(t, filter.Old))
	require.Equal(t, true, mustBool(t, filter.New))

	filters := resourceEvidence["filters"]
	require.Equal(t, MaxItemsOneTransitionUnknown, filters.Transition)
	require.Nil(t, filters.Old)
	require.Equal(t, false, mustBool(t, filters.New))
}

func TestFieldHistoryFixtureParity(t *testing.T) {
	t.Parallel()

	fixtures := []string{"parity-vsphere.json", "parity-keycloak-past.json"}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			metadata, err := ParseMetadata(mustReadFieldHistoryFixture(t, filepath.Join("metadata", fixture)))
			require.NoError(t, err)

			evidence := BuildFieldHistoryEvidence(metadata, metadata)
			require.NotNil(t, evidence.Resources)
			require.NotNil(t, evidence.Datasources)

			for _, tokenEvidence := range evidence.Resources {
				for _, pathEvidence := range tokenEvidence {
					expected := ClassifyMaxItemsOneTransition(pathEvidence.Old, pathEvidence.New)
					require.Equal(t, expected, pathEvidence.Transition)
				}
			}
			for _, tokenEvidence := range evidence.Datasources {
				for _, pathEvidence := range tokenEvidence {
					expected := ClassifyMaxItemsOneTransition(pathEvidence.Old, pathEvidence.New)
					require.Equal(t, expected, pathEvidence.Transition)
				}
			}
		})
	}
}

func loadMaxItemsFixture(t *testing.T, name string) maxItemsFixture {
	t.Helper()

	path := filepath.Join("testdata", "maxitems", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var fixture maxItemsFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func mustReadFieldHistoryFixture(t *testing.T, rel string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", rel))
	require.NoError(t, err)
	return data
}

func collectBoolPaths(m map[string]*bool) []string {
	if len(m) == 0 {
		return nil
	}
	paths := make([]string, 0, len(m))
	for path := range m {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func boolPtr(v bool) *bool {
	copy := v
	return &copy
}

func mustBool(t *testing.T, v *bool) bool {
	t.Helper()
	require.NotNil(t, v)
	return *v
}
