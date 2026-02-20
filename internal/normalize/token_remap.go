package normalize

import (
	"sort"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

type TokenRemap struct {
	OldResourceTokenToCanonical   map[string]string
	NewResourceTokenToCanonical   map[string]string
	OldDataSourceTokenToCanonical map[string]string
	NewDataSourceTokenToCanonical map[string]string

	oldResourceCanonicalToTokens   map[string][]string
	newResourceCanonicalToTokens   map[string][]string
	oldDataSourceCanonicalToTokens map[string][]string
	newDataSourceCanonicalToTokens map[string][]string
}

func BuildTokenRemap(oldMetadata, newMetadata *MetadataEnvelope) TokenRemap {
	remap := TokenRemap{
		OldResourceTokenToCanonical:    map[string]string{},
		NewResourceTokenToCanonical:    map[string]string{},
		OldDataSourceTokenToCanonical:  map[string]string{},
		NewDataSourceTokenToCanonical:  map[string]string{},
		oldResourceCanonicalToTokens:   map[string][]string{},
		newResourceCanonicalToTokens:   map[string][]string{},
		oldDataSourceCanonicalToTokens: map[string][]string{},
		newDataSourceCanonicalToTokens: map[string][]string{},
	}

	builder := tokenRemapBuilder{out: &remap}
	builder.buildKind(scopeResources, readHistoryMap(oldMetadata, true), readHistoryMap(newMetadata, true), parseResourceToken)
	builder.buildKind(scopeDataSources, readHistoryMap(oldMetadata, false), readHistoryMap(newMetadata, false), parseDataSourceToken)

	return remap
}

func (r TokenRemap) CanonicalForOld(scope, token string) (string, bool) {
	byScope := r.oldByScope(scope)
	canonical, ok := byScope[token]
	return canonical, ok
}

func (r TokenRemap) CanonicalForNew(scope, token string) (string, bool) {
	byScope := r.newByScope(scope)
	canonical, ok := byScope[token]
	return canonical, ok
}

func (r TokenRemap) OldTokensForCanonical(scope, canonical string) []string {
	byScope := r.oldCanonicalByScope(scope)
	return cloneSlice(byScope[canonical])
}

func (r TokenRemap) NewTokensForCanonical(scope, canonical string) []string {
	byScope := r.newCanonicalByScope(scope)
	return cloneSlice(byScope[canonical])
}

func (r TokenRemap) oldByScope(scope string) map[string]string {
	switch scope {
	case scopeResources:
		return r.OldResourceTokenToCanonical
	case scopeDataSources:
		return r.OldDataSourceTokenToCanonical
	default:
		return map[string]string{}
	}
}

func (r TokenRemap) newByScope(scope string) map[string]string {
	switch scope {
	case scopeResources:
		return r.NewResourceTokenToCanonical
	case scopeDataSources:
		return r.NewDataSourceTokenToCanonical
	default:
		return map[string]string{}
	}
}

func (r TokenRemap) oldCanonicalByScope(scope string) map[string][]string {
	switch scope {
	case scopeResources:
		return r.oldResourceCanonicalToTokens
	case scopeDataSources:
		return r.oldDataSourceCanonicalToTokens
	default:
		return map[string][]string{}
	}
}

func (r TokenRemap) newCanonicalByScope(scope string) map[string][]string {
	switch scope {
	case scopeResources:
		return r.newResourceCanonicalToTokens
	case scopeDataSources:
		return r.newDataSourceCanonicalToTokens
	default:
		return map[string][]string{}
	}
}

type tokenRemapBuilder struct {
	out *TokenRemap
}

type tokenEntry struct {
	snapshot string
	current  string
	tokens   []string
}

func (b tokenRemapBuilder) buildKind(
	scope string,
	oldMap map[string]*TokenHistory,
	newMap map[string]*TokenHistory,
	parseToken func(string) error,
) {
	entries := []tokenEntry{}
	uf := newUnionFind()

	entries = append(entries, b.buildSnapshotEntries("old", oldMap, parseToken, uf)...)
	entries = append(entries, b.buildSnapshotEntries("new", newMap, parseToken, uf)...)

	componentTokens := map[string][]string{}
	for _, token := range uf.tokens() {
		root := uf.find(token)
		componentTokens[root] = append(componentTokens[root], token)
	}

	type componentState struct {
		oldTokens   map[string]struct{}
		newTokens   map[string]struct{}
		oldCurrents map[string]struct{}
		newCurrents map[string]struct{}
	}
	components := map[string]*componentState{}
	for root := range componentTokens {
		components[root] = &componentState{
			oldTokens:   map[string]struct{}{},
			newTokens:   map[string]struct{}{},
			oldCurrents: map[string]struct{}{},
			newCurrents: map[string]struct{}{},
		}
	}

	for _, entry := range entries {
		if len(entry.tokens) == 0 {
			continue
		}
		root := uf.find(entry.tokens[0])
		state := components[root]
		for _, token := range entry.tokens {
			if entry.snapshot == "old" {
				state.oldTokens[token] = struct{}{}
			} else {
				state.newTokens[token] = struct{}{}
			}
		}
		if entry.current != "" {
			if entry.snapshot == "old" {
				state.oldCurrents[entry.current] = struct{}{}
			} else {
				state.newCurrents[entry.current] = struct{}{}
			}
		}
	}

	roots := sortedKeys(componentTokens)
	for _, root := range roots {
		toks := uniqueSorted(componentTokens[root])
		state := components[root]
		canonical := selectCanonical(toks, state.oldCurrents, state.newCurrents)

		oldByScope := b.out.oldByScope(scope)
		newByScope := b.out.newByScope(scope)
		for token := range state.oldTokens {
			oldByScope[token] = canonical
		}
		for token := range state.newTokens {
			newByScope[token] = canonical
		}
	}

	b.rebuildReverseLookupMaps()
}

func (b tokenRemapBuilder) buildSnapshotEntries(
	snapshot string,
	history map[string]*TokenHistory,
	parseToken func(string) error,
	uf *unionFind,
) []tokenEntry {
	entries := make([]tokenEntry, 0, len(history))
	for _, tfToken := range sortedKeys(history) {
		h := history[tfToken]
		if h == nil {
			continue
		}

		current := ""
		tokensForEntry := []string{}
		if err := parseToken(h.Current); err == nil {
			current = h.Current
			tokensForEntry = append(tokensForEntry, h.Current)
			uf.add(h.Current)
		}

		for _, alias := range h.Past {
			if err := parseToken(alias.Name); err != nil {
				continue
			}
			tokensForEntry = append(tokensForEntry, alias.Name)
			uf.add(alias.Name)
		}

		tokensForEntry = uniqueSorted(tokensForEntry)
		if len(tokensForEntry) > 1 {
			anchor := tokensForEntry[0]
			for _, tok := range tokensForEntry[1:] {
				uf.union(anchor, tok)
			}
		}

		entries = append(entries, tokenEntry{
			snapshot: snapshot,
			current:  current,
			tokens:   tokensForEntry,
		})
	}

	return entries
}

func (b tokenRemapBuilder) rebuildReverseLookupMaps() {
	b.out.oldResourceCanonicalToTokens = invertCanonicalMap(b.out.OldResourceTokenToCanonical)
	b.out.newResourceCanonicalToTokens = invertCanonicalMap(b.out.NewResourceTokenToCanonical)
	b.out.oldDataSourceCanonicalToTokens = invertCanonicalMap(b.out.OldDataSourceTokenToCanonical)
	b.out.newDataSourceCanonicalToTokens = invertCanonicalMap(b.out.NewDataSourceTokenToCanonical)
}

func parseResourceToken(s string) error {
	_, err := tokens.ParseTypeToken(s)
	return err
}

func parseDataSourceToken(s string) error {
	_, err := tokens.ParseModuleMember(s)
	return err
}

func readHistoryMap(metadata *MetadataEnvelope, resources bool) map[string]*TokenHistory {
	if metadata == nil || metadata.AutoAliasing == nil {
		return nil
	}
	if resources {
		return metadata.AutoAliasing.Resources
	}
	return metadata.AutoAliasing.Datasources
}

func selectCanonical(componentTokens []string, oldCurrents, newCurrents map[string]struct{}) string {
	if len(newCurrents) == 1 {
		return onlyKey(newCurrents)
	}
	if len(oldCurrents) == 1 {
		return onlyKey(oldCurrents)
	}
	if len(componentTokens) == 0 {
		return ""
	}
	return componentTokens[0]
}

func onlyKey(m map[string]struct{}) string {
	for k := range m {
		return k
	}
	return ""
}

type unionFind struct {
	parent map[string]string
	rank   map[string]int
}

func newUnionFind() *unionFind {
	return &unionFind{
		parent: map[string]string{},
		rank:   map[string]int{},
	}
}

func (u *unionFind) add(x string) {
	if _, ok := u.parent[x]; ok {
		return
	}
	u.parent[x] = x
	u.rank[x] = 0
}

func (u *unionFind) find(x string) string {
	if x == "" {
		return ""
	}
	p, ok := u.parent[x]
	if !ok {
		u.add(x)
		return x
	}
	if p != x {
		u.parent[x] = u.find(p)
	}
	return u.parent[x]
}

func (u *unionFind) union(a, b string) {
	ra := u.find(a)
	rb := u.find(b)
	if ra == rb {
		return
	}
	if u.rank[ra] < u.rank[rb] {
		u.parent[ra] = rb
		return
	}
	if u.rank[ra] > u.rank[rb] {
		u.parent[rb] = ra
		return
	}
	u.parent[rb] = ra
	u.rank[ra]++
}

func (u *unionFind) tokens() []string {
	return sortedKeys(u.parent)
}

func invertCanonicalMap(m map[string]string) map[string][]string {
	out := map[string][]string{}
	for token, canonical := range m {
		out[canonical] = append(out[canonical], token)
	}
	for canonical := range out {
		sort.Strings(out[canonical])
	}
	return out
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		set[v] = struct{}{}
	}
	return sortedKeys(set)
}

func sortedKeys[T any](m map[string]T) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func cloneSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
