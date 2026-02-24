package normalize

import (
	"maps"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// clonePackageSpec creates a copy-on-write snapshot for maps that Normalize may
// mutate so callers do not observe in-place map updates.
func clonePackageSpec(spec schema.PackageSpec) schema.PackageSpec {
	cloned := spec
	cloned.Resources = maps.Clone(spec.Resources)
	cloned.Functions = maps.Clone(spec.Functions)
	cloned.Types = maps.Clone(spec.Types)
	return cloned
}
