package normalize

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewNormalizationContextStrictMetadataRequired(t *testing.T) {
	t.Parallel()

	metadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget"
				}
			}
		}
	}`)

	t.Run("missing new metadata", func(t *testing.T) {
		t.Parallel()
		_, err := NewNormalizationContext(metadata, nil)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrResolverStrictMetadataRequired))

		var strictErr *StrictMetadataRequiredError
		require.ErrorAs(t, err, &strictErr)
		require.False(t, strictErr.MissingOld)
		require.True(t, strictErr.MissingNew)
	})

	t.Run("missing both metadata", func(t *testing.T) {
		t.Parallel()
		_, err := NewNormalizationContext(nil, nil)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrResolverStrictMetadataRequired))

		var strictErr *StrictMetadataRequiredError
		require.ErrorAs(t, err, &strictErr)
		require.True(t, strictErr.MissingOld)
		require.True(t, strictErr.MissingNew)
	})
}

func TestNewNormalizationContextBuildsRemapAndFieldEvidence(t *testing.T) {
	t.Parallel()

	old := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": true}
					}
				}
			}
		}
	}`)
	new := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widgetV2:Widget",
					"past": [{"name":"pkg:index/widget:Widget","inCodegen":true,"majorVersion":1}],
					"fields": {
						"list": {"maxItemsOne": false}
					}
				}
			}
		}
	}`)

	resolver, err := NewNormalizationContext(old, new)
	require.NoError(t, err)

	oldCanonical, ok := resolver.tokenRemap.CanonicalForOld(scopeResources, "pkg:index/widget:Widget")
	require.True(t, ok)
	require.Equal(t, "pkg:index/widgetV2:Widget", oldCanonical)

	newCanonical, ok := resolver.tokenRemap.CanonicalForNew(scopeResources, "pkg:index/widgetV2:Widget")
	require.True(t, ok)
	require.Equal(t, "pkg:index/widgetV2:Widget", newCanonical)

	evidence := resolver.fieldEvidence.Resources["pkg_widget"]
	require.NotNil(t, evidence)
	require.Equal(t, MaxItemsOneTransitionChanged, evidence["list"].Transition)
}

func TestResolverScopeCollision(t *testing.T) {
	t.Parallel()

	old := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/shared:Shared"
				}
			},
			"datasources": {
				"pkg_widget_ds": {
					"current": "pkg:index/shared:Shared"
				}
			}
		}
	}`)
	new := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/sharedResourceV2:Shared",
					"past": [{"name":"pkg:index/shared:Shared","inCodegen":true,"majorVersion":1}]
				}
			},
			"datasources": {
				"pkg_widget_ds": {
					"current": "pkg:index/sharedDataSourceV2:Shared",
					"past": [{"name":"pkg:index/shared:Shared","inCodegen":true,"majorVersion":1}]
				}
			}
		}
	}`)

	resolver, err := NewNormalizationContext(old, new)
	require.NoError(t, err)

	resourceCanonical, ok := resolver.tokenRemap.CanonicalForOld(scopeResources, "pkg:index/shared:Shared")
	require.True(t, ok)
	require.Equal(t, "pkg:index/sharedResourceV2:Shared", resourceCanonical)

	datasourceCanonical, ok := resolver.tokenRemap.CanonicalForOld(scopeDataSources, "pkg:index/shared:Shared")
	require.True(t, ok)
	require.Equal(t, "pkg:index/sharedDataSourceV2:Shared", datasourceCanonical)
}

func mustParseMetadataJSON(t *testing.T, raw string) *MetadataEnvelope {
	t.Helper()
	metadata, err := ParseMetadata([]byte(raw))
	require.NoError(t, err)
	return metadata
}
