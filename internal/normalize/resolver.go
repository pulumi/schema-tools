package normalize

import (
	"errors"
	"fmt"
)

var (
	// ErrResolverStrictMetadataRequired indicates strict-mode normalization cannot
	// proceed without both old and new metadata payloads.
	ErrResolverStrictMetadataRequired = errors.New("resolver strict mode metadata required")
)

// NormalizationContext bundles precomputed remap + field evidence used during
// schema normalization.
type NormalizationContext struct {
	tokenRemap    TokenRemap
	fieldEvidence FieldHistoryEvidence
}

// NewNormalizationContext constructs strict normalization evidence. Both sides
// must provide metadata.
func NewNormalizationContext(oldMetadata, newMetadata *MetadataEnvelope) (*NormalizationContext, error) {
	if oldMetadata == nil || newMetadata == nil {
		return nil, fmt.Errorf("%w: %s", ErrResolverStrictMetadataRequired, missingMetadataLabel(oldMetadata == nil, newMetadata == nil))
	}
	if err := ValidateMetadata(oldMetadata); err != nil {
		return nil, fmt.Errorf("old metadata: %w", err)
	}
	if err := ValidateMetadata(newMetadata); err != nil {
		return nil, fmt.Errorf("new metadata: %w", err)
	}

	return &NormalizationContext{
		tokenRemap:    BuildTokenRemap(oldMetadata, newMetadata),
		fieldEvidence: BuildFieldHistoryEvidence(oldMetadata, newMetadata),
	}, nil
}

func missingMetadataLabel(missingOld, missingNew bool) string {
	switch {
	case missingOld && missingNew:
		return "missing old and new metadata"
	case missingOld:
		return "missing old metadata"
	case missingNew:
		return "missing new metadata"
	default:
		return "metadata required"
	}
}
