package normalize

import "github.com/pulumi/pulumi/pkg/v3/codegen/schema"

// clonePackageSpec creates a copy-on-write snapshot for maps that Normalize may
// mutate so callers do not observe in-place map updates.
func clonePackageSpec(spec schema.PackageSpec) schema.PackageSpec {
	cloned := spec
	cloned.Resources = cloneMap(spec.Resources)
	cloned.Functions = cloneMap(spec.Functions)
	cloned.Types = cloneMap(spec.Types)
	return cloned
}

func cloneMap[T any](m map[string]T) map[string]T {
	if m == nil {
		return nil
	}
	out := make(map[string]T, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
