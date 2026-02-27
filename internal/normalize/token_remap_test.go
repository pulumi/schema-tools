package normalize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type tokenLookupFixture struct {
	Old   MetadataEnvelope      `json:"old"`
	New   MetadataEnvelope      `json:"new"`
	Cases []tokenLookupTestCase `json:"cases"`
}

type tokenLookupTestCase struct {
	Name       string   `json:"name"`
	Direction  string   `json:"direction"`
	Scope      string   `json:"scope"`
	Token      string   `json:"token"`
	Outcome    string   `json:"outcome"`
	Resolved   string   `json:"resolved"`
	Candidates []string `json:"candidates"`
}

func TestTokenLookupFixtures(t *testing.T) {
	t.Parallel()

	fixtures := []string{
		"single-rename.json",
		"multi-hop.json",
		"ambiguity-conflict.json",
	}

	for _, fixtureName := range fixtures {
		fixtureName := fixtureName
		t.Run(fixtureName, func(t *testing.T) {
			t.Parallel()

			fixture := loadTokenLookupFixture(t, fixtureName)

			for _, tc := range fixture.Cases {
				tc := tc
				t.Run(tc.Name, func(t *testing.T) {
					t.Parallel()

					var result TokenLookupResult
					switch tc.Direction {
					case "old":
						result = ResolveToken(&fixture.Old, &fixture.New, tc.Scope, tc.Token)
					case "new":
						result = ResolveNewToken(&fixture.Old, &fixture.New, tc.Scope, tc.Token)
					default:
						t.Fatalf("unsupported direction %q", tc.Direction)
					}

					require.Equal(t, TokenLookupOutcome(tc.Outcome), result.Outcome)
					require.Equal(t, tc.Resolved, result.Token)
					require.Equal(t, tc.Candidates, result.Candidates)
				})
			}
		})
	}
}

func TestTokenLookupMissingEvidence(t *testing.T) {
	t.Parallel()

	require.Equal(t, TokenLookupOutcomeNone, ResolveToken(nil, nil, scopeResources, "pkg:index/missing:Missing").Outcome)
	require.Equal(t, TokenLookupOutcomeNone, ResolveNewToken(nil, nil, scopeResources, "pkg:index/missing:Missing").Outcome)
	require.Equal(t, TokenLookupOutcomeNone, ResolveToken(nil, nil, "unknown", "pkg:index/missing:Missing").Outcome)
}

func TestTokenLookupSupportsTypeScope(t *testing.T) {
	t.Parallel()

	oldMetadata := &MetadataEnvelope{
		AutoAliasing: &AutoAliasing{
			Types: map[string]*TokenHistory{
				"pkg_widget_spec": {Current: "pkg:index/v1:WidgetSpec"},
			},
		},
	}
	newMetadata := &MetadataEnvelope{
		AutoAliasing: &AutoAliasing{
			Types: map[string]*TokenHistory{
				"pkg_widget_spec": {
					Current: "pkg:index/v2:WidgetSpec",
					Past: []TokenAlias{
						{Name: "pkg:index/v1:WidgetSpec"},
					},
				},
			},
		},
	}

	require.Equal(
		t,
		TokenLookupResult{
			Outcome:    TokenLookupOutcomeResolved,
			Token:      "pkg:index/v2:WidgetSpec",
			Candidates: []string{},
		},
		ResolveToken(oldMetadata, newMetadata, scopeTypes, "pkg:index/v1:WidgetSpec"),
	)
	require.Equal(
		t,
		TokenLookupResult{
			Outcome:    TokenLookupOutcomeResolved,
			Token:      "pkg:index/v1:WidgetSpec",
			Candidates: []string{},
		},
		ResolveNewToken(oldMetadata, newMetadata, scopeTypes, "pkg:index/v2:WidgetSpec"),
	)
}

func TestTokenLookupDirectionalityRegression(t *testing.T) {
	t.Parallel()

	oldMetadata := &MetadataEnvelope{
		AutoAliasing: &AutoAliasing{
			Resources: map[string]*TokenHistory{
				"pkg_shared": {
					Current: "pkg:index/sharedV1:Shared",
				},
				"pkg_old_only": {
					Current: "pkg:index/oldOnly:OldOnly",
				},
			},
		},
	}
	newMetadata := &MetadataEnvelope{
		AutoAliasing: &AutoAliasing{
			Resources: map[string]*TokenHistory{
				"pkg_shared": {
					Current: "pkg:index/sharedV2:Shared",
					Past: []TokenAlias{
						{Name: "pkg:index/sharedV1:Shared"},
					},
				},
				"pkg_new_only": {
					Current: "pkg:index/newOnly:NewOnly",
				},
			},
		},
	}

	require.Equal(
		t,
		TokenLookupResult{Outcome: TokenLookupOutcomeNone, Candidates: []string{}},
		ResolveToken(oldMetadata, newMetadata, scopeResources, "pkg:index/newOnly:NewOnly"),
	)

	require.Equal(
		t,
		TokenLookupResult{Outcome: TokenLookupOutcomeNone, Candidates: []string{}},
		ResolveNewToken(oldMetadata, newMetadata, scopeResources, "pkg:index/oldOnly:OldOnly"),
	)
}

func TestTokenLookupDeterministicRepeatedCalls(t *testing.T) {
	t.Parallel()

	oldMetadata := &MetadataEnvelope{
		AutoAliasing: &AutoAliasing{
			Resources: map[string]*TokenHistory{
				"pkg_widget": {
					Current: "pkg:index/widget:Widget",
				},
			},
		},
	}
	newMetadata := &MetadataEnvelope{
		AutoAliasing: &AutoAliasing{
			Resources: map[string]*TokenHistory{
				"pkg_widget": {
					Current: "pkg:index/widgetV2:Widget",
					Past: []TokenAlias{
						{Name: "pkg:index/widget:Widget"},
					},
				},
			},
		},
	}

	first := ResolveToken(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget")
	require.Equal(
		t,
		TokenLookupResult{
			Outcome:    TokenLookupOutcomeResolved,
			Token:      "pkg:index/widgetV2:Widget",
			Candidates: []string{},
		},
		first,
	)

	second := ResolveToken(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget")
	require.Equal(t, first, second)
}

func TestTokenLookupReturnedCandidatesAreDefensivelyCopied(t *testing.T) {
	t.Parallel()

	fixture := loadTokenLookupFixture(t, "ambiguity-conflict.json")

	token := "pkg:index/widgetLegacy:Widget"
	first := ResolveToken(&fixture.Old, &fixture.New, scopeResources, token)
	require.Equal(t, TokenLookupOutcomeAmbiguous, first.Outcome)
	require.Equal(t, []string{
		"pkg:index/widgetA_v2:WidgetA",
		"pkg:index/widgetB_v2:WidgetB",
	}, first.Candidates)

	first.Candidates[0] = "pkg:index/mutated:Mutated"

	second := ResolveToken(&fixture.Old, &fixture.New, scopeResources, token)
	require.Equal(t, TokenLookupOutcomeAmbiguous, second.Outcome)
	require.Equal(t, []string{
		"pkg:index/widgetA_v2:WidgetA",
		"pkg:index/widgetB_v2:WidgetB",
	}, second.Candidates)
}

func loadTokenLookupFixture(t *testing.T, name string) tokenLookupFixture {
	t.Helper()

	path := filepath.Join("testdata", "remap", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var fixture tokenLookupFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}
