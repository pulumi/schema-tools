package normalize

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestResolveFieldSourceSideEvidenceGating(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"name": {"maxItemsOne": false}
					}
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widgetV2:Widget",
					"past": [{"name":"pkg:index/widget:Widget","inCodegen":true,"majorVersion":1}],
					"fields": {
						"name": {"maxItemsOne": false},
						"names": {"maxItemsOne": true}
					}
				}
			}
		}
	}`)

	resolved := ResolveField(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget", "name")
	require.Equal(t, FieldLookupResult{
		Outcome:    TokenLookupOutcomeResolved,
		Field:      "name",
		Transition: MaxItemsOneTransitionUnchanged,
		Candidates: []string{},
	}, resolved)

	missingSourceField := ResolveField(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget", "names")
	require.Equal(t, FieldLookupResult{Outcome: TokenLookupOutcomeNone, Candidates: []string{}}, missingSourceField)

	unknownScope := ResolveField(oldMetadata, newMetadata, "unknown", "pkg:index/widget:Widget", "name")
	require.Equal(t, FieldLookupResult{Outcome: TokenLookupOutcomeNone, Candidates: []string{}}, unknownScope)
}

func TestResolveFieldAmbiguousCandidatesDeterministic(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget_a": {
					"current": "pkg:index/widgetA:WidgetA",
					"past": [{"name":"pkg:index/legacy:Widget","inCodegen":true,"majorVersion":1}],
					"fields": {
						"spec": {"maxItemsOne": true}
					}
				},
				"pkg_widget_b": {
					"current": "pkg:index/widgetB:WidgetB",
					"past": [{"name":"pkg:index/legacy:Widget","inCodegen":true,"majorVersion":1}],
					"fields": {
						"spec": {"maxItemsOne": true}
					}
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget_a": {
					"current": "pkg:index/widgetA_v2:WidgetA",
					"fields": {
						"spec": {"maxItemsOne": false}
					}
				},
				"pkg_widget_b": {
					"current": "pkg:index/widgetB_v2:WidgetB",
					"fields": {
						"config": {"maxItemsOne": false}
					}
				}
			}
		}
	}`)

	result := ResolveField(oldMetadata, newMetadata, scopeResources, "pkg:index/legacy:Widget", "spec")
	require.Equal(t, TokenLookupOutcomeAmbiguous, result.Outcome)
	require.Equal(t, []string{"spec"}, result.Candidates)

	result.Candidates[0] = "mutated"
	again := ResolveField(oldMetadata, newMetadata, scopeResources, "pkg:index/legacy:Widget", "spec")
	require.Equal(t, TokenLookupOutcomeAmbiguous, again.Outcome)
	require.Equal(t, []string{"spec"}, again.Candidates)
}

func TestResolveFieldSingleUnrelatedKnownTargetFieldNoop(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"spec": {"maxItemsOne": true}
					}
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widgetV2:Widget",
					"past": [{"name":"pkg:index/widget:Widget","inCodegen":true,"majorVersion":1}],
					"fields": {
						"config": {"maxItemsOne": false}
					}
				}
			}
		}
	}`)

	result := ResolveField(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget", "spec")
	require.Equal(t, FieldLookupResult{Outcome: TokenLookupOutcomeNone, Candidates: []string{}}, result)
}

func TestResolveNewFieldDirectionalityGating(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_shared": {
					"current": "pkg:index/sharedV1:Shared",
					"fields": {
						"name": {"maxItemsOne": false}
					}
				},
				"pkg_old_only": {
					"current": "pkg:index/oldOnly:OldOnly",
					"fields": {
						"name": {"maxItemsOne": false}
					}
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_shared": {
					"current": "pkg:index/sharedV2:Shared",
					"past": [{"name":"pkg:index/sharedV1:Shared","inCodegen":true,"majorVersion":1}],
					"fields": {
						"name": {"maxItemsOne": false}
					}
				},
				"pkg_new_only": {
					"current": "pkg:index/newOnly:NewOnly",
					"fields": {
						"name": {"maxItemsOne": false}
					}
				}
			}
		}
	}`)

	require.Equal(
		t,
		FieldLookupResult{Outcome: TokenLookupOutcomeNone, Candidates: []string{}},
		ResolveField(oldMetadata, newMetadata, scopeResources, "pkg:index/newOnly:NewOnly", "name"),
	)

	require.Equal(
		t,
		FieldLookupResult{Outcome: TokenLookupOutcomeNone, Candidates: []string{}},
		ResolveNewField(oldMetadata, newMetadata, scopeResources, "pkg:index/oldOnly:OldOnly", "name"),
	)
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

func mustParseMetadataJSON(t *testing.T, raw string) *MetadataEnvelope {
	t.Helper()
	metadata, err := ParseMetadata([]byte(raw))
	require.NoError(t, err)
	return metadata
}
