package normalize

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/schema-tools/compare"
	"github.com/stretchr/testify/require"
)

func TestNormalizeStrictMissingMetadataFailure(t *testing.T) {
	t.Parallel()

	_, err := Normalize(schema.PackageSpec{}, schema.PackageSpec{}, nil, nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrResolverStrictMetadataRequired))

	var strictErr *StrictMetadataRequiredError
	require.ErrorAs(t, err, &strictErr)
	require.True(t, strictErr.MissingOld)
	require.True(t, strictErr.MissingNew)
}

func TestNormalizeRewritesRenamedResourceAndReportsRename(t *testing.T) {
	t.Parallel()

	oldToken := "pkg:index:Widget"
	newToken := "pkg:index:RenamedWidget"

	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			oldToken: {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			newToken: {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "integer"}},
				},
			},
		},
	}

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget"
				}
			},
			"datasources": {}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:RenamedWidget",
					"past": [{"name":"pkg:index:Widget","inCodegen":true,"majorVersion":1}]
				}
			},
			"datasources": {}
		}
	}`)

	normalized, err := Normalize(oldSchema, newSchema, oldMetadata, newMetadata)
	require.NoError(t, err)

	require.Equal(t, []TokenRename{{
		Scope:    scopeResources,
		OldToken: oldToken,
		NewToken: newToken,
	}}, normalized.Renames)
	require.Contains(t, normalized.NewSchema.Resources, oldToken)
	require.NotContains(t, normalized.NewSchema.Resources, newToken)
	require.Equal(t, "integer", normalized.NewSchema.Resources[oldToken].InputProperties["name"].TypeSpec.Type)

	result := compare.Schemas(normalized.OldSchema, normalized.NewSchema, compare.Options{
		Provider:   "pkg",
		MaxChanges: -1,
	})
	require.Empty(t, result.NewResources)
	require.Equal(t, 0, normalizeSummaryCount(result.Summary, "missing-resource"))
	require.Equal(t, 1, normalizeSummaryCount(result.Summary, "type-changed"))
}

func TestNormalizeScopeTokensSkipsRenameTargetCollision(t *testing.T) {
	t.Parallel()

	oldMap := map[string]int{
		"pkg:index:legacy:Widget": 1,
	}
	newMap := map[string]int{
		"pkg:index:modern:Widget": 10,
		"pkg:index:legacy:Widget": 20,
	}

	oldCanonical := func(_ string, token string) (string, bool) {
		switch token {
		case "pkg:index:legacy:Widget":
			return "canonical-widget", true
		default:
			return "", false
		}
	}
	newCanonical := func(_ string, token string) (string, bool) {
		switch token {
		case "pkg:index:modern:Widget":
			return "canonical-widget", true
		case "pkg:index:legacy:Widget":
			return "canonical-reused-token", true
		default:
			return "", false
		}
	}

	normalized, renames := normalizeScopeTokens(scopeResources, oldMap, newMap, oldCanonical, newCanonical)
	require.Equal(t, map[string]int{
		"pkg:index:legacy:Widget": 20,
		"pkg:index:modern:Widget": 10,
	}, normalized)
	require.Empty(t, renames)
}

func TestNormalizeRewritesMaxItemsOneTypeChangeAndReportsChange(t *testing.T) {
	t.Parallel()

	token := "pkg:index:Widget"
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"filter": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"filter": {
						TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Type: "string"},
						},
					},
				},
			},
		},
	}

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {
						"filter": {"maxItemsOne": true}
					}
				}
			},
			"datasources": {}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {
						"filter": {"maxItemsOne": false}
					}
				}
			},
			"datasources": {}
		}
	}`)

	normalized, err := Normalize(oldSchema, newSchema, oldMetadata, newMetadata)
	require.NoError(t, err)

	require.Equal(t, []MaxItemsOneChange{{
		Scope:    scopeResources,
		Token:    token,
		Location: "inputs",
		Field:    "filter",
		OldType:  "string",
		NewType:  "array",
	}}, normalized.MaxItemsOne)

	require.Equal(
		t,
		"string",
		normalized.NewSchema.Resources[token].InputProperties["filter"].TypeSpec.Type,
	)

	result := compare.Schemas(normalized.OldSchema, normalized.NewSchema, compare.Options{
		Provider:   "pkg",
		MaxChanges: -1,
	})
	require.Equal(t, 0, normalizeSummaryCount(result.Summary, "type-changed"))
}

func TestNormalizeMaxItemsOneSharedTypeSkipsNestedRewrite(t *testing.T) {
	t.Parallel()

	tokenA := "pkg:index:WidgetA"
	tokenB := "pkg:index:WidgetB"
	sharedTypeToken := "pkg:index:SharedConfig"

	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			sharedTypeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"filter": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			tokenA: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + sharedTypeToken}},
				},
			},
			tokenB: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + sharedTypeToken}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			sharedTypeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"filter": {
							TypeSpec: schema.TypeSpec{
								Type:  "array",
								Items: &schema.TypeSpec{Type: "string"},
							},
						},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			tokenA: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + sharedTypeToken}},
				},
			},
			tokenB: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + sharedTypeToken}},
				},
			},
		},
	}

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget_a": {
					"current": "pkg:index:WidgetA",
					"fields": {
						"config": {
							"fields": {
								"filter": {"maxItemsOne": true}
							}
						}
					}
				},
				"pkg_widget_b": {
					"current": "pkg:index:WidgetB"
				}
			},
			"datasources": {}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget_a": {
					"current": "pkg:index:WidgetA",
					"fields": {
						"config": {
							"fields": {
								"filter": {"maxItemsOne": false}
							}
						}
					}
				},
				"pkg_widget_b": {
					"current": "pkg:index:WidgetB"
				}
			},
			"datasources": {}
		}
	}`)

	normalized, err := Normalize(oldSchema, newSchema, oldMetadata, newMetadata)
	require.NoError(t, err)
	require.Empty(t, normalized.MaxItemsOne)

	refA := normalized.NewSchema.Resources[tokenA].InputProperties["config"].TypeSpec.Ref
	refB := normalized.NewSchema.Resources[tokenB].InputProperties["config"].TypeSpec.Ref
	require.Equal(t, refA, refB)
	require.Equal(t, "#/types/"+sharedTypeToken, refB)
	require.Equal(t, "array", normalized.NewSchema.Types[sharedTypeToken].Properties["filter"].TypeSpec.Type)

	result := compare.Schemas(normalized.OldSchema, normalized.NewSchema, compare.Options{
		Provider:   "pkg",
		MaxChanges: -1,
	})
	require.GreaterOrEqual(t, normalizeSummaryCount(result.Summary, "type-changed"), 1)
}

func TestNormalizeMaxItemsOneFunctionIOPaths(t *testing.T) {
	t.Parallel()

	token := "pkg:index:getWidget"
	inputTypeToken := "pkg:index:RuleInput"
	outputTypeToken := "pkg:index:RuleOutput"
	functionSpec := schema.FunctionSpec{
		Inputs: &schema.ObjectTypeSpec{
			Properties: map[string]schema.PropertySpec{
				"rules": {
					TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Ref: "#/types/" + inputTypeToken},
					},
				},
			},
		},
		Outputs: &schema.ObjectTypeSpec{
			Properties: map[string]schema.PropertySpec{
				"results": {
					TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Ref: "#/types/" + outputTypeToken},
					},
				},
			},
		},
	}

	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			inputTypeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
			outputTypeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			token: functionSpec,
		},
	}
	newSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			inputTypeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {
							TypeSpec: schema.TypeSpec{
								Type:  "array",
								Items: &schema.TypeSpec{Type: "string"},
							},
						},
					},
				},
			},
			outputTypeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {
							TypeSpec: schema.TypeSpec{
								Type:  "array",
								Items: &schema.TypeSpec{Type: "string"},
							},
						},
					},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			token: functionSpec,
		},
	}

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {},
			"datasources": {
				"pkg_get_widget": {
					"current": "pkg:index:getWidget",
					"fields": {
						"rules": {
							"elem": {
								"fields": {
									"value": {"maxItemsOne": true}
								}
							}
						},
						"results": {
							"elem": {
								"fields": {
									"value": {"maxItemsOne": true}
								}
							}
						}
					}
				}
			}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {},
			"datasources": {
				"pkg_get_widget": {
					"current": "pkg:index:getWidget",
					"fields": {
						"rules": {
							"elem": {
								"fields": {
									"value": {"maxItemsOne": false}
								}
							}
						},
						"results": {
							"elem": {
								"fields": {
									"value": {"maxItemsOne": false}
								}
							}
						}
					}
				}
			}
		}
	}`)

	normalized, err := Normalize(oldSchema, newSchema, oldMetadata, newMetadata)
	require.NoError(t, err)
	require.Len(t, normalized.MaxItemsOne, 2)
	require.Equal(t, scopeDataSources, normalized.MaxItemsOne[0].Scope)
	require.Equal(t, token, normalized.MaxItemsOne[0].Token)
	require.Equal(t, "rules[*].value", normalized.MaxItemsOne[0].Field)
	require.Equal(t, "inputs", normalized.MaxItemsOne[0].Location)
	require.Equal(t, "results[*].value", normalized.MaxItemsOne[1].Field)
	require.Equal(t, "outputs", normalized.MaxItemsOne[1].Location)

	inputRef := normalized.NewSchema.Functions[token].Inputs.Properties["rules"].TypeSpec.Items.Ref
	outputRef := normalized.NewSchema.Functions[token].Outputs.Properties["results"].TypeSpec.Items.Ref
	normalizedInputType := strings.TrimPrefix(inputRef, "#/types/")
	normalizedOutputType := strings.TrimPrefix(outputRef, "#/types/")
	require.Equal(t, "string", normalized.NewSchema.Types[normalizedInputType].Properties["value"].TypeSpec.Type)
	require.Equal(t, "string", normalized.NewSchema.Types[normalizedOutputType].Properties["value"].TypeSpec.Type)

	result := compare.Schemas(normalized.OldSchema, normalized.NewSchema, compare.Options{
		Provider:   "pkg",
		MaxChanges: -1,
	})
	require.Equal(t, 0, normalizeSummaryCount(result.Summary, "type-changed"))
}

func TestNormalizeDoesNotMutateCallerOwnedNewSchema(t *testing.T) {
	t.Parallel()

	token := "pkg:index:Widget"
	sharedTypeToken := "pkg:index:Config"
	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			sharedTypeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"filter": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + sharedTypeToken}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			sharedTypeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"filter": {
							TypeSpec: schema.TypeSpec{
								Type:  "array",
								Items: &schema.TypeSpec{Type: "string"},
							},
						},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + sharedTypeToken}},
				},
			},
		},
	}
	before := mustMarshalPackageSpec(t, newSchema)

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {
						"config": {
							"fields": {
								"filter": {"maxItemsOne": true}
							}
						}
					}
				}
			},
			"datasources": {}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {
						"config": {
							"fields": {
								"filter": {"maxItemsOne": false}
							}
						}
					}
				}
			},
			"datasources": {}
		}
	}`)

	normalized, err := Normalize(oldSchema, newSchema, oldMetadata, newMetadata)
	require.NoError(t, err)
	require.Equal(
		t,
		"string",
		normalized.NewSchema.Types[sharedTypeToken].Properties["filter"].TypeSpec.Type,
	)
	require.Equal(t, before, mustMarshalPackageSpec(t, newSchema))
	require.Equal(
		t,
		"array",
		newSchema.Types[sharedTypeToken].Properties["filter"].TypeSpec.Type,
	)
}

func TestNormalizePreservesPropertyDefaultConcreteType(t *testing.T) {
	t.Parallel()

	token := "pkg:index:Widget"
	oldSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"filter": {TypeSpec: schema.TypeSpec{Type: "string"}},
					"count": {
						TypeSpec: schema.TypeSpec{Type: "integer"},
						Default:  7,
					},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"filter": {
						TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Type: "string"},
						},
					},
					"count": {
						TypeSpec: schema.TypeSpec{Type: "integer"},
						Default:  7,
					},
				},
			},
		},
	}

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {"filter": {"maxItemsOne": true}}
				}
			},
			"datasources": {}
		}
	}`)
	newMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {"filter": {"maxItemsOne": false}}
				}
			},
			"datasources": {}
		}
	}`)

	normalized, err := Normalize(oldSchema, newSchema, oldMetadata, newMetadata)
	require.NoError(t, err)

	defaultValue := normalized.NewSchema.Resources[token].InputProperties["count"].Default
	_, ok := defaultValue.(int)
	require.True(t, ok, "expected int default, got %T", defaultValue)
}

func normalizeSummaryCount(summary []compare.SummaryItem, category string) int {
	for _, item := range summary {
		if item.Category == category {
			return item.Count
		}
	}
	return 0
}

func mustMarshalPackageSpec(t *testing.T, spec schema.PackageSpec) string {
	t.Helper()

	data, err := json.Marshal(spec)
	require.NoError(t, err)
	return string(data)
}
