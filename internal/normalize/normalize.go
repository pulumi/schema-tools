package normalize

import (
	"sort"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// Result contains normalized schemas and emitted normalization evidence.
type Result struct {
	OldSchema   schema.PackageSpec
	NewSchema   schema.PackageSpec
	Renames     []TokenRename
	MaxItemsOne []MaxItemsOneChange
}

// Normalize applies strict metadata-first normalization to the new schema and
// returns injected-difference evidence for compare output.
func Normalize(
	oldSchema, newSchema schema.PackageSpec,
	oldMetadata, newMetadata *MetadataEnvelope,
) (Result, error) {
	normCtx, err := NewNormalizationContext(oldMetadata, newMetadata)
	if err != nil {
		return Result{}, err
	}

	normalizedNew := clonePackageSpec(newSchema)
	renames := []TokenRename{}

	normalizedResources, resourceRenames := normalizeScopeTokens(
		scopeResources,
		oldSchema.Resources,
		newSchema.Resources,
		normCtx.tokenRemap.CanonicalForOld,
		normCtx.tokenRemap.CanonicalForNew,
	)
	normalizedFunctions, functionRenames := normalizeScopeTokens(
		scopeDataSources,
		oldSchema.Functions,
		newSchema.Functions,
		normCtx.tokenRemap.CanonicalForOld,
		normCtx.tokenRemap.CanonicalForNew,
	)
	normalizedNew.Resources = normalizedResources
	normalizedNew.Functions = normalizedFunctions

	renames = append(renames, resourceRenames...)
	renames = append(renames, functionRenames...)
	renames = append(renames, detectInCodegenAliasTransitions(scopeResources, oldMetadata, newMetadata)...)
	renames = append(renames, detectInCodegenAliasTransitions(scopeDataSources, oldMetadata, newMetadata)...)
	sort.Slice(renames, func(i, j int) bool {
		if renames[i].Scope != renames[j].Scope {
			return renames[i].Scope < renames[j].Scope
		}
		if renames[i].Kind != renames[j].Kind {
			return renames[i].Kind < renames[j].Kind
		}
		if renames[i].OldToken != renames[j].OldToken {
			return renames[i].OldToken < renames[j].OldToken
		}
		return renames[i].NewToken < renames[j].NewToken
	})

	maxItemsOne := applyMaxItemsOneNormalization(oldSchema, &normalizedNew, normCtx, oldMetadata, newMetadata)
	sort.Slice(maxItemsOne, func(i, j int) bool {
		if maxItemsOne[i].Scope != maxItemsOne[j].Scope {
			return maxItemsOne[i].Scope < maxItemsOne[j].Scope
		}
		if maxItemsOne[i].Token != maxItemsOne[j].Token {
			return maxItemsOne[i].Token < maxItemsOne[j].Token
		}
		if maxItemsOne[i].Location != maxItemsOne[j].Location {
			return maxItemsOne[i].Location < maxItemsOne[j].Location
		}
		return maxItemsOne[i].Field < maxItemsOne[j].Field
	})

	return Result{
		OldSchema:   oldSchema,
		NewSchema:   normalizedNew,
		Renames:     renames,
		MaxItemsOne: maxItemsOne,
	}, nil
}

// detectInCodegenAliasTransitions identifies aliases that remain in metadata as
// in-codegen compat entries and emits informational rename evidence.
func detectInCodegenAliasTransitions(
	scope string,
	oldMetadata, newMetadata *MetadataEnvelope,
) []TokenRename {
	oldHistoryByTFToken := readHistoryMap(oldMetadata, scope == scopeResources)
	newHistoryByTFToken := readHistoryMap(newMetadata, scope == scopeResources)
	if len(oldHistoryByTFToken) == 0 || len(newHistoryByTFToken) == 0 {
		return nil
	}

	validateToken := parseDataSourceToken
	if scope == scopeResources {
		validateToken = parseResourceToken
	}

	seen := map[string]struct{}{}
	renames := []TokenRename{}
	for _, tfToken := range sortedKeys(newHistoryByTFToken) {
		newHistory := newHistoryByTFToken[tfToken]
		oldHistory := oldHistoryByTFToken[tfToken]
		if newHistory == nil || oldHistory == nil {
			continue
		}
		newCurrent := strings.TrimSpace(newHistory.Current)
		if newCurrent == "" || validateToken(newCurrent) != nil {
			continue
		}

		oldKnownTokens := historyTokenSet(oldHistory)
		for _, alias := range newHistory.Past {
			aliasToken := strings.TrimSpace(alias.Name)
			if aliasToken == "" || aliasToken == newCurrent {
				continue
			}
			if !alias.InCodegen || alias.MajorVersion != newHistory.MajorVersion {
				continue
			}
			if validateToken(aliasToken) != nil {
				continue
			}
			if _, existedOld := oldKnownTokens[aliasToken]; !existedOld {
				continue
			}

			key := scope + "|" + aliasToken + "|" + newCurrent
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			renames = append(renames, TokenRename{
				Scope:    scope,
				OldToken: aliasToken,
				NewToken: newCurrent,
				Kind:     TokenRenameKindInCodegenAlias,
			})
		}
	}
	return renames
}

// historyTokenSet returns current + past token names for one TF token history.
func historyTokenSet(history *TokenHistory) map[string]struct{} {
	tokens := map[string]struct{}{}
	if history == nil {
		return tokens
	}
	if current := strings.TrimSpace(history.Current); current != "" {
		tokens[current] = struct{}{}
	}
	for _, alias := range history.Past {
		if token := strings.TrimSpace(alias.Name); token != "" {
			tokens[token] = struct{}{}
		}
	}
	return tokens
}

// normalizeScopeTokens rewrites new-map token keys to old canonical keys when
// metadata provides an unambiguous 1:1 rename mapping.
func normalizeScopeTokens[T any](
	scope string,
	oldMap map[string]T,
	newMap map[string]T,
	oldCanonical func(string, string) (string, bool),
	newCanonical func(string, string) (string, bool),
) (map[string]T, []TokenRename) {
	if newMap == nil {
		return nil, nil
	}

	oldByCanonical := map[string][]string{}
	for token := range oldMap {
		canonical, ok := oldCanonical(scope, token)
		if !ok {
			continue
		}
		oldByCanonical[canonical] = append(oldByCanonical[canonical], token)
	}
	for canonical := range oldByCanonical {
		oldByCanonical[canonical] = uniqueSorted(oldByCanonical[canonical])
	}

	newByCanonical := map[string][]string{}
	for token := range newMap {
		canonical, ok := newCanonical(scope, token)
		if !ok {
			continue
		}
		newByCanonical[canonical] = append(newByCanonical[canonical], token)
	}
	for canonical := range newByCanonical {
		newByCanonical[canonical] = uniqueSorted(newByCanonical[canonical])
	}

	renameTargets := map[string]string{}
	renames := []TokenRename{}
	for _, canonical := range sortedKeys(oldByCanonical) {
		newTokens, ok := newByCanonical[canonical]
		if !ok {
			continue
		}
		oldTokens := oldByCanonical[canonical]
		if len(oldTokens) != 1 || len(newTokens) != 1 {
			continue
		}
		oldToken, newToken := oldTokens[0], newTokens[0]
		if oldToken == newToken {
			continue
		}
		// If the rename target already exists in the new schema map for another
		// canonical component, do not remap and risk dropping entries.
		if _, exists := newMap[oldToken]; exists {
			continue
		}
		renameTargets[newToken] = oldToken
		renames = append(renames, TokenRename{
			Scope:    scope,
			OldToken: oldToken,
			NewToken: newToken,
		})
	}

	normalized := make(map[string]T, len(newMap))
	for _, token := range sortedKeys(newMap) {
		value := newMap[token]
		target, ok := renameTargets[token]
		if !ok {
			target = token
		}
		normalized[target] = value
	}

	return normalized, renames
}
