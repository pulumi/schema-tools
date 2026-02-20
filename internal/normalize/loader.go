package normalize

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func LoadMetadata(path string) (*MetadataEnvelope, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%w: empty path", ErrMetadataRequired)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Join(ErrMetadataRequired, err)
	}

	return ParseMetadata(data)
}

func ParseMetadata(data []byte) (*MetadataEnvelope, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%w: empty payload", ErrMetadataRequired)
	}

	var metadata MetadataEnvelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMetadataInvalid, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing content", ErrMetadataInvalid)
	}

	if err := ValidateMetadata(&metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

func ValidateMetadata(metadata *MetadataEnvelope) error {
	if metadata == nil || metadata.AutoAliasing == nil {
		return fmt.Errorf("%w: missing auto-aliasing payload", ErrMetadataRequired)
	}

	if metadata.AutoAliasing.Version != nil && *metadata.AutoAliasing.Version != SupportedAutoAliasingVersion {
		return fmt.Errorf("%w: expected %d got %d", ErrMetadataVersionUnsupported,
			SupportedAutoAliasingVersion, *metadata.AutoAliasing.Version)
	}

	if err := validateTokenHistoryMap("resources", metadata.AutoAliasing.Resources); err != nil {
		return err
	}
	if err := validateTokenHistoryMap("datasources", metadata.AutoAliasing.Datasources); err != nil {
		return err
	}

	return nil
}

func validateTokenHistoryMap(kind string, m map[string]*TokenHistory) error {
	for tfToken, history := range m {
		if history == nil {
			return fmt.Errorf("%w: %s[%q] must not be null", ErrMetadataInvalid, kind, tfToken)
		}

		if strings.TrimSpace(history.Current) == "" {
			return fmt.Errorf("%w: %s[%q].current must be set", ErrMetadataInvalid, kind, tfToken)
		}

		for i, past := range history.Past {
			if strings.TrimSpace(past.Name) == "" {
				return fmt.Errorf("%w: %s[%q].past[%d].name must be set", ErrMetadataInvalid, kind, tfToken, i)
			}
		}

		if err := validateFieldHistoryMap(kind, tfToken, history.Fields); err != nil {
			return err
		}
	}

	return nil
}

func validateFieldHistoryMap(kind, tfToken string, fields map[string]*FieldHistory) error {
	for fieldName, history := range fields {
		if history == nil {
			return fmt.Errorf("%w: %s[%q].fields[%q] must not be null", ErrMetadataInvalid, kind, tfToken, fieldName)
		}
		if err := validateFieldHistoryNode(history, fmt.Sprintf("%s[%q].fields[%q]", kind, tfToken, fieldName)); err != nil {
			return err
		}
	}

	return nil
}

func validateFieldHistoryNode(history *FieldHistory, path string) error {
	if history == nil {
		return fmt.Errorf("%w: %s must not be null", ErrMetadataInvalid, path)
	}

	for fieldName, child := range history.Fields {
		if child == nil {
			return fmt.Errorf("%w: %s.fields[%q] must not be null", ErrMetadataInvalid, path, fieldName)
		}
		if err := validateFieldHistoryNode(child, fmt.Sprintf("%s.fields[%q]", path, fieldName)); err != nil {
			return err
		}
	}

	if history.Elem != nil {
		if err := validateFieldHistoryNode(history.Elem, path+".elem"); err != nil {
			return err
		}
	}

	return nil
}
