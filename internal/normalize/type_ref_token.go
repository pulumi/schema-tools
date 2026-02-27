package normalize

import (
	"net/url"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

const typeRefPrefix = "#/types/"

// ParseTypeRefToken extracts and validates a Pulumi type token from a local
// schema ref (for example "#/types/pkg:module/name:Type").
func ParseTypeRefToken(ref string) (tokens.Type, bool) {
	if !strings.HasPrefix(ref, typeRefPrefix) {
		return "", false
	}

	token := strings.TrimSpace(strings.TrimPrefix(ref, typeRefPrefix))
	if token == "" {
		return "", false
	}

	unescaped, err := url.PathUnescape(token)
	if err != nil || unescaped == "" {
		return "", false
	}

	typ, err := tokens.ParseTypeToken(unescaped)
	if err != nil {
		return "", false
	}
	return typ, true
}
