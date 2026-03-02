package normalize

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTypeRefToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ref   string
		token string
		ok    bool
	}{
		{
			name:  "valid local ref",
			ref:   "#/types/pkg:index:Widget",
			token: "pkg:index:Widget",
			ok:    true,
		},
		{
			name:  "valid escaped module path",
			ref:   "#/types/pkg:index%2Fv2:Widget",
			token: "pkg:index/v2:Widget",
			ok:    true,
		},
		{
			name: "invalid missing prefix",
			ref:  "pkg:index:Widget",
			ok:   false,
		},
		{
			name: "invalid empty token",
			ref:  "#/types/ ",
			ok:   false,
		},
		{
			name: "invalid malformed token",
			ref:  "#/types/not-a-type-token",
			ok:   false,
		},
		{
			name: "invalid bad escape",
			ref:  "#/types/pkg:index%2:Widget",
			ok:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			token, ok := ParseTypeRefToken(tt.ref)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.token, token.String())
		})
	}
}
