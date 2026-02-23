package normalize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type remapFixture struct {
	Old      MetadataEnvelope `json:"old"`
	New      MetadataEnvelope `json:"new"`
	Expected struct {
		Old map[string]string `json:"old"`
		New map[string]string `json:"new"`
	} `json:"expected"`
}

func TestTokenRemap(t *testing.T) {
	t.Parallel()

	fixtures := []string{"no-rename.json", "single-rename.json", "multi-hop.json"}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()

			f := loadRemapFixture(t, fixture)
			remap := BuildTokenRemap(&f.Old, &f.New)
			expectedOldResources, expectedOldDatasources := expectedByScope(&f, true)
			expectedNewResources, expectedNewDatasources := expectedByScope(&f, false)

			require.Equal(t, expectedOldResources, remap.OldResourceTokenToCanonical)
			require.Equal(t, expectedOldDatasources, remap.OldDataSourceTokenToCanonical)
			require.Equal(t, expectedNewResources, remap.NewResourceTokenToCanonical)
			require.Equal(t, expectedNewDatasources, remap.NewDataSourceTokenToCanonical)

			for token, canonical := range expectedOldResources {
				resolved, ok := remap.CanonicalForOld(scopeResources, token)
				require.True(t, ok)
				require.Equal(t, canonical, resolved)
				require.Contains(t, remap.OldTokensForCanonical(scopeResources, canonical), token)
			}
			for token, canonical := range expectedOldDatasources {
				resolved, ok := remap.CanonicalForOld(scopeDataSources, token)
				require.True(t, ok)
				require.Equal(t, canonical, resolved)
				require.Contains(t, remap.OldTokensForCanonical(scopeDataSources, canonical), token)
			}
			for token, canonical := range expectedNewResources {
				resolved, ok := remap.CanonicalForNew(scopeResources, token)
				require.True(t, ok)
				require.Equal(t, canonical, resolved)
				require.Contains(t, remap.NewTokensForCanonical(scopeResources, canonical), token)
			}
			for token, canonical := range expectedNewDatasources {
				resolved, ok := remap.CanonicalForNew(scopeDataSources, token)
				require.True(t, ok)
				require.Equal(t, canonical, resolved)
				require.Contains(t, remap.NewTokensForCanonical(scopeDataSources, canonical), token)
			}
		})
	}
}

func TestAliasHistory(t *testing.T) {
	t.Parallel()

	fixture := loadRemapFixture(t, "conflict-cycle.json")
	remap := BuildTokenRemap(&fixture.Old, &fixture.New)

	require.Equal(t, map[string]string{
		"pkg:index/widgetA:WidgetA":     "pkg:index/widgetA:WidgetA",
		"pkg:index/widgetB:WidgetB":     "pkg:index/widgetA:WidgetA",
		"pkg:index/widgetLegacy:Widget": "pkg:index/widgetA:WidgetA",
	}, remap.OldResourceTokenToCanonical)
	require.Equal(t, map[string]string{
		"pkg:index/getWidgetA:getWidgetA": "pkg:index/getWidgetA:getWidgetA",
		"pkg:index/getWidgetB:getWidgetB": "pkg:index/getWidgetA:getWidgetA",
	}, remap.OldDataSourceTokenToCanonical)
	require.Equal(t, map[string]string{}, remap.NewResourceTokenToCanonical)
	require.Equal(t, map[string]string{}, remap.NewDataSourceTokenToCanonical)

	require.Equal(t, []string{
		"pkg:index/widgetA:WidgetA",
		"pkg:index/widgetB:WidgetB",
		"pkg:index/widgetLegacy:Widget",
	}, remap.OldTokensForCanonical(scopeResources, "pkg:index/widgetA:WidgetA"))
	require.Equal(t, []string{
		"pkg:index/getWidgetA:getWidgetA",
		"pkg:index/getWidgetB:getWidgetB",
	}, remap.OldTokensForCanonical(scopeDataSources, "pkg:index/getWidgetA:getWidgetA"))
	require.Nil(t, remap.OldTokensForCanonical(scopeResources, "pkg:index/missing:Missing"))
	require.Nil(t, remap.NewTokensForCanonical(scopeResources, "pkg:index/widgetA:WidgetA"))

	resourceLegacyCanonical, ok := remap.CanonicalForOld(scopeResources, "pkg:index/widgetLegacy:Widget")
	require.True(t, ok)
	require.Equal(t, "pkg:index/widgetA:WidgetA", resourceLegacyCanonical)

	datasourceCanonicalA, ok := remap.CanonicalForOld(scopeDataSources, "pkg:index/getWidgetA:getWidgetA")
	require.True(t, ok)
	require.Equal(t, "pkg:index/getWidgetA:getWidgetA", datasourceCanonicalA)

	datasourceCanonicalB, ok := remap.CanonicalForOld(scopeDataSources, "pkg:index/getWidgetB:getWidgetB")
	require.True(t, ok)
	require.Equal(t, "pkg:index/getWidgetA:getWidgetA", datasourceCanonicalB)

}

func TestTokenRemapScopeCollision(t *testing.T) {
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

	remap := BuildTokenRemap(old, new)

	oldResourceCanonical, ok := remap.CanonicalForOld(scopeResources, "pkg:index/shared:Shared")
	require.True(t, ok)
	require.Equal(t, "pkg:index/sharedResourceV2:Shared", oldResourceCanonical)

	oldDataSourceCanonical, ok := remap.CanonicalForOld(scopeDataSources, "pkg:index/shared:Shared")
	require.True(t, ok)
	require.Equal(t, "pkg:index/sharedDataSourceV2:Shared", oldDataSourceCanonical)

	require.NotEqual(t, oldResourceCanonical, oldDataSourceCanonical)
}

func TestBridgeSemanticParity(t *testing.T) {
	t.Parallel()

	valid := loadRemapFixture(t, "single-rename.json")
	validRemap := BuildTokenRemap(&valid.Old, &valid.New)
	require.NotEmpty(t, validRemap.OldResourceTokenToCanonical)

	invalid := &MetadataEnvelope{
		AutoAliasing: &AutoAliasing{
			Resources: map[string]*TokenHistory{
				"bad_resource": {
					Current: "not-a-resource-token",
				},
			},
			Datasources: map[string]*TokenHistory{
				"bad_datasource": {
					Current: "not-a-datasource-token",
				},
			},
		},
	}

	invalidRemap := BuildTokenRemap(invalid, nil)
	require.Empty(t, invalidRemap.OldResourceTokenToCanonical)
	require.Empty(t, invalidRemap.OldDataSourceTokenToCanonical)
}

func loadRemapFixture(t *testing.T, name string) remapFixture {
	t.Helper()

	path := filepath.Join("testdata", "remap", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var fixture remapFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func expectedByScope(fixture *remapFixture, old bool) (map[string]string, map[string]string) {
	resources := map[string]string{}
	datasources := map[string]string{}
	if fixture == nil || fixture.Old.AutoAliasing == nil || fixture.New.AutoAliasing == nil {
		return resources, datasources
	}
	convert := func(entries map[string]*TokenHistory, expected map[string]string, dst map[string]string) {
		for _, history := range entries {
			if history == nil {
				continue
			}
			canonical, ok := expected[history.Current]
			if !ok {
				continue
			}
			dst[history.Current] = canonical
			for _, alias := range history.Past {
				if aliasCanonical, ok := expected[alias.Name]; ok {
					dst[alias.Name] = aliasCanonical
				}
			}
		}
	}
	if old {
		convert(fixture.Old.AutoAliasing.Resources, fixture.Expected.Old, resources)
		convert(fixture.Old.AutoAliasing.Datasources, fixture.Expected.Old, datasources)
		return resources, datasources
	}
	convert(fixture.New.AutoAliasing.Resources, fixture.Expected.New, resources)
	convert(fixture.New.AutoAliasing.Datasources, fixture.Expected.New, datasources)
	return resources, datasources
}
