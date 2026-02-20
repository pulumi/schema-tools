package normalize

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/require"
)

func TestIsMaxItemsOneTypeChangeRequiresFullTypeSpecMatch(t *testing.T) {
	t.Parallel()

	base := schema.TypeSpec{
		Type: "object",
		AdditionalProperties: &schema.TypeSpec{
			Type: "string",
		},
		OneOf: []schema.TypeSpec{
			{Type: "string"},
			{Type: "integer"},
		},
		Discriminator: &schema.DiscriminatorSpec{
			PropertyName: "kind",
			Mapping: map[string]string{
				"a": "#/types/pkg:index:A",
			},
		},
		Plain: true,
	}

	t.Run("equivalent scalar and array item normalize", func(t *testing.T) {
		t.Parallel()
		oldType := base
		newType := schema.TypeSpec{
			Type:  "array",
			Items: cloneTypeSpecPtr(base),
		}
		require.True(t, isMaxItemsOneTypeChange(&oldType, &newType))
	})

	t.Run("additionalProperties mismatch does not normalize", func(t *testing.T) {
		t.Parallel()
		oldType := base
		changed := base
		changed.AdditionalProperties = &schema.TypeSpec{Type: "integer"}
		newType := schema.TypeSpec{Type: "array", Items: cloneTypeSpecPtr(changed)}
		require.False(t, isMaxItemsOneTypeChange(&oldType, &newType))
	})

	t.Run("oneOf mismatch does not normalize", func(t *testing.T) {
		t.Parallel()
		oldType := base
		changed := base
		changed.OneOf = []schema.TypeSpec{{Type: "string"}}
		newType := schema.TypeSpec{Type: "array", Items: cloneTypeSpecPtr(changed)}
		require.False(t, isMaxItemsOneTypeChange(&oldType, &newType))
	})

	t.Run("discriminator mismatch does not normalize", func(t *testing.T) {
		t.Parallel()
		oldType := base
		changed := base
		changed.Discriminator = &schema.DiscriminatorSpec{
			PropertyName: "kind2",
		}
		newType := schema.TypeSpec{Type: "array", Items: cloneTypeSpecPtr(changed)}
		require.False(t, isMaxItemsOneTypeChange(&oldType, &newType))
	})

	t.Run("plain mismatch does not normalize", func(t *testing.T) {
		t.Parallel()
		oldType := base
		changed := base
		changed.Plain = false
		newType := schema.TypeSpec{Type: "array", Items: cloneTypeSpecPtr(changed)}
		require.False(t, isMaxItemsOneTypeChange(&oldType, &newType))
	})
}

func TestNormalizeMaxItemsOneRecursiveTypeNotTreatedAsShared(t *testing.T) {
	t.Parallel()

	token := "pkg:index:Widget"
	typeToken := "pkg:index:Node"
	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
						"next":  {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {
							TypeSpec: schema.TypeSpec{
								Type:  "array",
								Items: &schema.TypeSpec{Type: "string"},
							},
						},
						"next": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
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
						"config": {
							"fields": {
								"value": {"maxItemsOne": true}
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
								"value": {"maxItemsOne": false}
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
	require.Len(t, normalized.MaxItemsOne, 1)
	require.Equal(t, "string", normalized.NewSchema.Types[typeToken].Properties["value"].TypeSpec.Type)
}

func TestNormalizeMaxItemsOneSkipsWhenTypeSharedViaStateInputs(t *testing.T) {
	t.Parallel()

	token := "pkg:index:Widget"
	typeToken := "pkg:index:Config"
	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
				},
				StateInputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"lookup": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
					},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
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
		Resources: oldSchema.Resources,
	}

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {
						"config": {
							"fields": {
								"value": {"maxItemsOne": true}
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
								"value": {"maxItemsOne": false}
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
	require.Empty(t, normalized.MaxItemsOne)
	require.Equal(t, "array", normalized.NewSchema.Types[typeToken].Properties["value"].TypeSpec.Type)
}

func TestNormalizeMaxItemsOneSkipsWhenTypeSharedViaFunctionReturnType(t *testing.T) {
	t.Parallel()

	token := "pkg:index:getWidget"
	typeToken := "pkg:index:Config"
	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			token: {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
					},
				},
				ReturnType: &schema.ReturnTypeSpec{
					TypeSpec: &schema.TypeSpec{Ref: "#/types/" + typeToken},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
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
		Functions: oldSchema.Functions,
	}

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {},
			"datasources": {
				"pkg_get_widget": {
					"current": "pkg:index:getWidget",
					"fields": {
						"config": {
							"fields": {
								"value": {"maxItemsOne": true}
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
						"config": {
							"fields": {
								"value": {"maxItemsOne": false}
							}
						}
					}
				}
			}
		}
	}`)

	normalized, err := Normalize(oldSchema, newSchema, oldMetadata, newMetadata)
	require.NoError(t, err)
	require.Empty(t, normalized.MaxItemsOne)
	require.Equal(t, "array", normalized.NewSchema.Types[typeToken].Properties["value"].TypeSpec.Type)
}

func TestNormalizeMaxItemsOneSkipsWhenTypeSharedViaProvider(t *testing.T) {
	t.Parallel()

	token := "pkg:index:Widget"
	typeToken := "pkg:index:Config"
	oldSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Provider: schema.ResourceSpec{
			InputProperties: map[string]schema.PropertySpec{
				"providerConfig": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
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
		Provider:  oldSchema.Provider,
		Resources: oldSchema.Resources,
	}

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {
						"config": {
							"fields": {
								"value": {"maxItemsOne": true}
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
								"value": {"maxItemsOne": false}
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
	require.Empty(t, normalized.MaxItemsOne)
	require.Equal(t, "array", normalized.NewSchema.Types[typeToken].Properties["value"].TypeSpec.Type)
}

func TestNormalizeMaxItemsOneSkipsWhenTypeSharedViaConfigVariables(t *testing.T) {
	t.Parallel()

	token := "pkg:index:Widget"
	typeToken := "pkg:index:Config"
	oldSchema := schema.PackageSpec{
		Config: schema.ConfigSpec{
			Variables: map[string]schema.PropertySpec{
				"defaultConfig": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
			},
		},
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Resources: map[string]schema.ResourceSpec{
			token: {
				InputProperties: map[string]schema.PropertySpec{
					"config": {TypeSpec: schema.TypeSpec{Ref: "#/types/" + typeToken}},
				},
			},
		},
	}
	newSchema := schema.PackageSpec{
		Config: oldSchema.Config,
		Types: map[string]schema.ComplexTypeSpec{
			typeToken: {
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
		Resources: oldSchema.Resources,
	}

	oldMetadata := mustParseMetadataJSON(t, `{
		"auto-aliasing": {
			"resources": {
				"pkg_widget": {
					"current": "pkg:index:Widget",
					"fields": {
						"config": {
							"fields": {
								"value": {"maxItemsOne": true}
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
								"value": {"maxItemsOne": false}
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
	require.Empty(t, normalized.MaxItemsOne)
	require.Equal(t, "array", normalized.NewSchema.Types[typeToken].Properties["value"].TypeSpec.Type)
}

func TestParseFieldPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []fieldPathPart
		ok       bool
	}{
		{
			name:  "simple",
			input: "config.value",
			expected: []fieldPathPart{
				{Name: "config", Elem: false},
				{Name: "value", Elem: false},
			},
			ok: true,
		},
		{
			name:  "array element",
			input: "rules[*].value",
			expected: []fieldPathPart{
				{Name: "rules", Elem: true},
				{Name: "value", Elem: false},
			},
			ok: true,
		},
		{
			name:  "whitespace trimmed",
			input: "  rules[*] . value  ",
			expected: []fieldPathPart{
				{Name: "rules", Elem: true},
				{Name: "value", Elem: false},
			},
			ok: true,
		},
		{name: "empty", input: "", ok: false},
		{name: "double dot", input: "a..b", ok: false},
		{name: "trailing dot", input: "a.", ok: false},
		{name: "dangling element", input: "[*]", ok: false},
		{name: "embedded element marker", input: "a[*]b", ok: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parts, ok := parseFieldPath(test.input)
			require.Equal(t, test.ok, ok)
			if test.ok {
				require.Equal(t, test.expected, parts)
			}
		})
	}
}
