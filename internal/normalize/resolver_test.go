package normalize

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveEquivalentTypeChangeResolvedEquivalent(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
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
	newMetadata := mustParseMetadataJSON(t, `{
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

	result := ResolveEquivalentTypeChange(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget", "list", "array<string>", "string")
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: true, Field: "list", Candidates: []string{}}, result)
}

func TestResolveEquivalentTypeChangeResolvedButNotEquivalent(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
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
	newMetadata := mustParseMetadataJSON(t, `{
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

	result := ResolveEquivalentTypeChange(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget", "list", "array<string>", "number")
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: false, Field: "list", Candidates: []string{}}, result)
}

func TestResolveEquivalentTypeChangeAmbiguous(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget_a": {
					"current": "pkg:index/widgetA:WidgetA",
					"past": [{"name":"pkg:index/legacy:Widget","inCodegen":true,"majorVersion":1}],
					"fields": {"spec": {"maxItemsOne": true}}
				},
				"pkg_widget_b": {
					"current": "pkg:index/widgetB:WidgetB",
					"past": [{"name":"pkg:index/legacy:Widget","inCodegen":true,"majorVersion":1}],
					"fields": {"spec": {"maxItemsOne": true}}
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget_a": {
					"current": "pkg:index/widgetA_v2:WidgetA",
					"fields": {"spec": {"maxItemsOne": false}}
				},
				"pkg_widget_b": {
					"current": "pkg:index/widgetB_v2:WidgetB",
					"fields": {"config": {"maxItemsOne": false}}
				}
			}
		}
	}`)

	result := ResolveEquivalentTypeChange(oldMetadata, newMetadata, scopeResources, "pkg:index/legacy:Widget", "spec", "array<string>", "string")
	require.Equal(t, TokenLookupOutcomeAmbiguous, result.Outcome)
	require.False(t, result.Equivalent)
	require.Equal(t, []string{"spec"}, result.Candidates)
}

func TestResolveEquivalentTypeChangeNoneNoop(t *testing.T) {
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
						"name": {"maxItemsOne": false}
					}
				}
			}
		}
	}`)

	result := ResolveEquivalentTypeChange(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget", "list", "array<string>", "string")
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeNone, Equivalent: false, Candidates: []string{}}, result)
}

func TestResolveNewEquivalentTypeChangeDirectionality(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
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
	newMetadata := mustParseMetadataJSON(t, `{
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

	result := ResolveNewEquivalentTypeChange(oldMetadata, newMetadata, scopeResources, "pkg:index/widgetV2:Widget", "list", "string", "array<string>")
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: true, Field: "list", Candidates: []string{}}, result)
}

func TestResolveEquivalentTypeChangeResolvedUnchangedTransitionNotEquivalent(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
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
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widgetV2:Widget",
					"past": [{"name":"pkg:index/widget:Widget","inCodegen":true,"majorVersion":1}],
					"fields": {
						"list": {"maxItemsOne": true}
					}
				}
			}
		}
	}`)

	result := ResolveEquivalentTypeChange(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget", "list", "array<string>", "string")
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: false, Field: "list", Candidates: []string{}}, result)
}

func TestResolveEquivalentTypeChangeResolvedUnknownTransitionNotEquivalent(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {}
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
						"list": {"maxItemsOne": false}
					}
				}
			}
		}
	}`)

	result := ResolveEquivalentTypeChange(oldMetadata, newMetadata, scopeResources, "pkg:index/widget:Widget", "list", "array<string>", "string")
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: false, Field: "list", Candidates: []string{}}, result)
}

func TestResolveEquivalentTypeChangeResolvedTypeRefRenameEquivalent(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": true}
					}
				}
			},
			"types": {
				"pkg_widget_spec": {
					"current": "pkg:index/v1:WidgetSpec"
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": false}
					}
				}
			},
			"types": {
				"pkg_widget_spec": {
					"current": "pkg:index/v2:WidgetSpec",
					"past": [{"name":"pkg:index/v1:WidgetSpec","inCodegen":false,"majorVersion":1}]
				}
			}
		}
	}`)

	result := ResolveEquivalentTypeChange(
		oldMetadata,
		newMetadata,
		scopeResources,
		"pkg:index/widget:Widget",
		"list",
		"array<#/types/pkg:index/v1:WidgetSpec>",
		"#/types/pkg:index/v2:WidgetSpec",
	)
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: true, Field: "list", Candidates: []string{}}, result)
}

func TestResolveEquivalentTypeChangeResolvedTypeRefRenameEquivalentEscapedRef(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": true}
					}
				}
			},
			"types": {
				"pkg_widget_spec": {
					"current": "pkg:index/v1:WidgetSpec"
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": false}
					}
				}
			},
			"types": {
				"pkg_widget_spec": {
					"current": "pkg:index/v2:WidgetSpec",
					"past": [{"name":"pkg:index/v1:WidgetSpec","inCodegen":false,"majorVersion":1}]
				}
			}
		}
	}`)

	result := ResolveEquivalentTypeChange(
		oldMetadata,
		newMetadata,
		scopeResources,
		"pkg:index/widget:Widget",
		"list",
		"array<#/types/pkg:index%2Fv1:WidgetSpec>",
		"#/types/pkg:index%2Fv2:WidgetSpec",
	)
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: true, Field: "list", Candidates: []string{}}, result)
}

func TestResolveEquivalentTypeChangeTypeRefRenameAmbiguousNotEquivalent(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": true}
					}
				}
			},
			"types": {
				"pkg_widget_spec": {
					"current": "pkg:index/v1:WidgetSpec"
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": false}
					}
				}
			},
			"types": {
				"pkg_widget_spec_a": {
					"current": "pkg:index/v2:WidgetSpecA",
					"past": [{"name":"pkg:index/v1:WidgetSpec","inCodegen":false,"majorVersion":1}]
				},
				"pkg_widget_spec_b": {
					"current": "pkg:index/v2:WidgetSpecB",
					"past": [{"name":"pkg:index/v1:WidgetSpec","inCodegen":false,"majorVersion":1}]
				}
			}
		}
	}`)

	result := ResolveEquivalentTypeChange(
		oldMetadata,
		newMetadata,
		scopeResources,
		"pkg:index/widget:Widget",
		"list",
		"array<#/types/pkg:index/v1:WidgetSpec>",
		"#/types/pkg:index/v2:WidgetSpecA",
	)
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: false, Field: "list", Candidates: []string{}}, result)
}

func TestResolveEquivalentTypeChangeTypeRefRenameNoEvidenceNotEquivalent(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": true}
					}
				}
			},
			"types": {
				"pkg_widget_spec": {
					"current": "pkg:index/v1:WidgetSpec"
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": false}
					}
				}
			},
			"types": {
				"pkg_other_spec": {
					"current": "pkg:index/v2:OtherSpec"
				}
			}
		}
	}`)

	result := ResolveEquivalentTypeChange(
		oldMetadata,
		newMetadata,
		scopeResources,
		"pkg:index/widget:Widget",
		"list",
		"array<#/types/pkg:index/v1:WidgetSpec>",
		"#/types/pkg:index/v2:WidgetSpec",
	)
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: false, Field: "list", Candidates: []string{}}, result)
}

func TestResolveEquivalentTypeChangeNestedTypeRefNotOverSuppressed(t *testing.T) {
	t.Parallel()

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": true}
					}
				}
			},
			"types": {
				"pkg_widget_spec": {
					"current": "pkg:index/v1:WidgetSpec"
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index/widget:Widget",
					"fields": {
						"list": {"maxItemsOne": false}
					}
				}
			},
			"types": {
				"pkg_widget_spec": {
					"current": "pkg:index/v2:WidgetSpec",
					"past": [{"name":"pkg:index/v1:WidgetSpec","inCodegen":false,"majorVersion":1}]
				}
			}
		}
	}`)

	result := ResolveEquivalentTypeChange(
		oldMetadata,
		newMetadata,
		scopeResources,
		"pkg:index/widget:Widget",
		"list",
		"array<map<#/types/pkg:index/v1:WidgetSpec>>",
		"map<#/types/pkg:index/v2:WidgetSpec>",
	)
	require.Equal(t, EquivalentTypeChangeResult{Outcome: TokenLookupOutcomeResolved, Equivalent: false, Field: "list", Candidates: []string{}}, result)
}

func TestParseTypeCardinalityEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		base    string
		isArray bool
		ok      bool
	}{
		{name: "empty", raw: "", ok: false},
		{name: "space", raw: "   ", ok: false},
		{name: "array empty", raw: "array<>", ok: false},
		{name: "array missing close", raw: "array<string", ok: true, base: "array<string", isArray: false},
		{name: "array spaced inner", raw: "array< string >", ok: true, base: "string", isArray: true},
		{name: "slice empty", raw: "[]", ok: false},
		{name: "slice spaced inner", raw: " string []", ok: true, base: "string", isArray: true},
		{name: "plain", raw: "number", ok: true, base: "number", isArray: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base, isArray, ok := parseTypeCardinality(tt.raw)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.base, base)
			require.Equal(t, tt.isArray, isArray)
		})
	}
}
