package normalize

import (
	"errors"
)

var (
	ErrResolverStrictMetadataRequired = errors.New("resolver strict mode metadata required")
)

type StrictMetadataRequiredError struct {
	MissingOld bool
	MissingNew bool
}

func (e *StrictMetadataRequiredError) Error() string {
	switch {
	case e.MissingOld && e.MissingNew:
		return "resolver strict mode metadata required: missing old and new metadata"
	case e.MissingOld:
		return "resolver strict mode metadata required: missing old metadata"
	case e.MissingNew:
		return "resolver strict mode metadata required: missing new metadata"
	default:
		return "resolver strict mode metadata required"
	}
}

func (e *StrictMetadataRequiredError) Unwrap() error {
	return ErrResolverStrictMetadataRequired
}

type NormalizationContext struct {
	tokenRemap    TokenRemap
	fieldEvidence FieldHistoryEvidence
}

func NewNormalizationContext(oldMetadata, newMetadata *MetadataEnvelope) (*NormalizationContext, error) {
	if oldMetadata == nil || newMetadata == nil {
		return nil, &StrictMetadataRequiredError{
			MissingOld: oldMetadata == nil,
			MissingNew: newMetadata == nil,
		}
	}

	return &NormalizationContext{
		tokenRemap:    BuildTokenRemap(oldMetadata, newMetadata),
		fieldEvidence: BuildFieldHistoryEvidence(oldMetadata, newMetadata),
	}, nil
}
