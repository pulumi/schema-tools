package normalize

import "sort"

const (
	scopeResources   = "resources"
	scopeDatasources = "datasources"
)

type tokenLookupDirection string

const (
	tokenLookupDirectionOldToNew tokenLookupDirection = "old-to-new"
	tokenLookupDirectionNewToOld tokenLookupDirection = "new-to-old"
)

// ResolveToken resolves an old-snapshot token to new-snapshot token evidence.
// Callers must treat old/new metadata as immutable during compare execution.
// Concurrent metadata mutation is not supported.
//
// Example:
//
//	result := ResolveToken(oldMeta, newMeta, "resources", "pkg:index/v1:Widget")
func ResolveToken(oldMetadata, newMetadata *MetadataEnvelope, scope, oldToken string) TokenLookupResult {
	return resolveToken(oldMetadata, newMetadata, tokenLookupDirectionOldToNew, scope, oldToken)
}

// ResolveNewToken resolves a new-snapshot token to old-snapshot token evidence.
// Callers must treat old/new metadata as immutable during compare execution.
// Concurrent metadata mutation is not supported.
//
// Example:
//
//	result := ResolveNewToken(oldMeta, newMeta, "resources", "pkg:index/v2:Widget")
func ResolveNewToken(oldMetadata, newMetadata *MetadataEnvelope, scope, newToken string) TokenLookupResult {
	return resolveToken(oldMetadata, newMetadata, tokenLookupDirectionNewToOld, scope, newToken)
}

// resolveToken is the shared resolver entrypoint for both direction wrappers.
func resolveToken(oldMetadata, newMetadata *MetadataEnvelope, direction tokenLookupDirection, scope, token string) TokenLookupResult {
	fromMap, toMap := resolveDirectionMaps(oldMetadata, newMetadata, direction, scope)
	return resolveTokenDirection(fromMap, toMap, token)
}

// resolveDirectionMaps returns the source and target history maps for a lookup direction.
func resolveDirectionMaps(oldMetadata, newMetadata *MetadataEnvelope, direction tokenLookupDirection, scope string) (map[string]*TokenHistory, map[string]*TokenHistory) {
	switch direction {
	case tokenLookupDirectionOldToNew:
		return readHistoryMap(oldMetadata, scope), readHistoryMap(newMetadata, scope)
	case tokenLookupDirectionNewToOld:
		return readHistoryMap(newMetadata, scope), readHistoryMap(oldMetadata, scope)
	default:
		return nil, nil
	}
}

// resolveTokenDirection applies source-evidence gating and computes deterministic candidates.
func resolveTokenDirection(fromMap, toMap map[string]*TokenHistory, token string) TokenLookupResult {
	if !tokenExistsInMap(fromMap, token) {
		return tokenLookupNoneResult()
	}

	candidateTokens := map[string]struct{}{}

	collectCurrentCandidates(fromMap, toMap, token, candidateTokens)
	collectCurrentCandidates(toMap, toMap, token, candidateTokens)

	candidates := sortedCandidateTokens(candidateTokens)
	switch len(candidates) {
	case 0:
		return tokenLookupNoneResult()
	case 1:
		return TokenLookupResult{
			Outcome:    TokenLookupOutcomeResolved,
			Token:      candidates[0],
			Candidates: []string{},
		}
	default:
		return TokenLookupResult{
			Outcome:    TokenLookupOutcomeAmbiguous,
			Candidates: candidates,
		}
	}
}

// tokenExistsInMap returns true when token appears in any current or past alias entry.
func tokenExistsInMap(history map[string]*TokenHistory, token string) bool {
	for _, entry := range history {
		if tokenAppearsInHistory(entry, token) {
			return true
		}
	}
	return false
}

// collectCurrentCandidates adds resolved current tokens for matching tf-token history entries.
func collectCurrentCandidates(fromMap, toMap map[string]*TokenHistory, token string, out map[string]struct{}) {
	for tfToken, entry := range fromMap {
		if !tokenAppearsInHistory(entry, token) {
			continue
		}
		current := currentToken(toMap[tfToken])
		if current == "" {
			continue
		}
		out[current] = struct{}{}
	}
}

// tokenAppearsInHistory checks whether token matches the current name or any past alias.
func tokenAppearsInHistory(entry *TokenHistory, token string) bool {
	if entry == nil || token == "" {
		return false
	}

	if entry.Current == token {
		return true
	}

	for _, past := range entry.Past {
		if past.Name == token {
			return true
		}
	}
	return false
}

// currentToken returns the current token name for a history entry.
func currentToken(entry *TokenHistory) string {
	if entry == nil {
		return ""
	}
	return entry.Current
}

// tokenLookupNoneResult returns a canonical "none" response with empty candidates.
func tokenLookupNoneResult() TokenLookupResult {
	return TokenLookupResult{
		Outcome:    TokenLookupOutcomeNone,
		Candidates: []string{},
	}
}

// sortedCandidateTokens returns candidate tokens in deterministic lexical order.
func sortedCandidateTokens(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// readHistoryMap selects the resource or datasource token history map by scope.
func readHistoryMap(metadata *MetadataEnvelope, scope string) map[string]*TokenHistory {
	if metadata == nil || metadata.AutoAliasing == nil {
		return nil
	}

	switch scope {
	case scopeResources:
		return metadata.AutoAliasing.Resources
	case scopeDatasources:
		return metadata.AutoAliasing.Datasources
	default:
		return nil
	}
}
