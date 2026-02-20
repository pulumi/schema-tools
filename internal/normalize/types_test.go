package normalize

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMetadataOptionality(t *testing.T) {
	t.Parallel()

	metadata, err := ParseMetadata([]byte(`{
		"auto-aliasing": {
			"resources": {
				"pkg_r1": {
					"current": "pkg:index/r1:R1",
					"fields": {
						"nested": {
							"elem": {
								"fields": {
									"leaf": {}
								}
							}
						}
					}
				}
			}
		}
	}`))
	require.NoError(t, err)

	entry := metadata.AutoAliasing.Resources["pkg_r1"]
	require.NotNil(t, entry)
	require.Empty(t, entry.Past)
	require.Zero(t, entry.MajorVersion)

	leaf := entry.Fields["nested"].Elem.Fields["leaf"]
	require.NotNil(t, leaf)
	require.Nil(t, leaf.MaxItemsOne)
}

func TestValidateMetadataUnknownFieldsTolerated(t *testing.T) {
	t.Parallel()

	metadata, err := ParseMetadata([]byte(`{
		"auto-aliasing": {
			"resources": {
				"pkg_r1": {
					"current": "pkg:index/r1:R1",
					"futureField": {
						"x": 1
					}
				}
			}
		},
		"futureEnvelope": true
	}`))
	require.NoError(t, err)
	require.NotNil(t, metadata)
}
