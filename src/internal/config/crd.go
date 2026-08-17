package config

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/kubara-io/libkubara/crdvalidate"
	"github.com/kubara-io/libkubara/manifest"

	"github.com/go-viper/mapstructure/v2"
)

const (
	KubaraConfigurationAPIVersion = "kubara.io/v1alpha5"
	KubaraConfigurationKind       = "KubaraConfiguration"
)

//go:embed crd/kubara.io_kubaraconfigurations.yaml
var kubaraConfigurationCRD []byte

var (
	kubaraConfigurationValidatorOnce sync.Once
	kubaraConfigurationValidator     *crdvalidate.Validator
	kubaraConfigurationValidatorErr  error
)

func ConfigurationCRD() []byte {
	return append([]byte(nil), kubaraConfigurationCRD...)
}

// ConfigurationJSONSchema returns the selected CRD version's root schema for
// editor validation of KubaraConfiguration documents.
func ConfigurationJSONSchema() (map[string]any, error) {
	return configurationJSONSchema(kubaraConfigurationCRD)
}

func configurationJSONSchema(crdData []byte) (map[string]any, error) {
	object, err := manifest.DecodeOne(bytes.NewReader(crdData))
	if err != nil {
		return nil, fmt.Errorf("decode KubaraConfiguration CRD: %w", err)
	}
	spec, ok := object.Data()["spec"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("KubaraConfiguration CRD has no spec")
	}
	versions, ok := spec["versions"].([]any)
	if !ok {
		return nil, fmt.Errorf("KubaraConfiguration CRD has no versions")
	}
	for _, version := range versions {
		versionMap, ok := version.(map[string]any)
		if !ok || versionMap["name"] != "v1alpha5" {
			continue
		}
		schema, ok := versionMap["schema"].(map[string]any)
		if !ok {
			break
		}
		root, ok := schema["openAPIV3Schema"].(map[string]any)
		if !ok {
			break
		}
		out := make(map[string]any, len(root)+4)
		for key, value := range root {
			out[key] = value
		}
		out["$schema"] = "https://json-schema.org/draft/2020-12/schema"
		out["$id"] = "https://kubara.io/schemas/kubaraconfiguration-v1alpha5.json"
		out["$defs"] = map[string]any{}
		out["title"] = KubaraConfigurationKind
		return out, nil
	}
	return nil, fmt.Errorf("KubaraConfiguration CRD has no v1alpha5 OpenAPI schema")
}

func configurationValidator() (*crdvalidate.Validator, error) {
	kubaraConfigurationValidatorOnce.Do(func() {
		definition, err := crdvalidate.DecodeCRD(bytes.NewReader(kubaraConfigurationCRD))
		if err != nil {
			kubaraConfigurationValidatorErr = fmt.Errorf("decode embedded KubaraConfiguration CRD: %w", err)
			return
		}
		kubaraConfigurationValidator, kubaraConfigurationValidatorErr = crdvalidate.Compile(definition)
		if kubaraConfigurationValidatorErr != nil {
			kubaraConfigurationValidatorErr = fmt.Errorf("compile embedded KubaraConfiguration CRD: %w", kubaraConfigurationValidatorErr)
		}
	})
	return kubaraConfigurationValidator, kubaraConfigurationValidatorErr
}

func decodeKubaraConfiguration(data []byte) (*Config, error) {
	return decodeKubaraConfigurationWithCRD(data, kubaraConfigurationCRD)
}

func decodeKubaraConfigurationWithCRD(data, crdData []byte) (*Config, error) {
	object, err := manifest.DecodeOne(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode KubaraConfiguration manifest: %w", err)
	}
	validator, err := compileConfigurationCRD(crdData)
	if err != nil {
		return nil, err
	}

	result := validator.ValidateCreate(context.Background(), object, crdvalidate.RejectUnknown)
	if !result.Valid() {
		return nil, fmt.Errorf("validate KubaraConfiguration %q: %w", object.Name(), result.Err())
	}
	return configFromKubaraConfiguration(result.Object)
}

func compileConfigurationCRD(data []byte) (*crdvalidate.Validator, error) {
	if bytes.Equal(data, kubaraConfigurationCRD) {
		return configurationValidator()
	}
	definition, err := crdvalidate.DecodeCRD(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode catalog-pinned KubaraConfiguration CRD: %w", err)
	}
	validator, err := crdvalidate.Compile(definition)
	if err != nil {
		return nil, fmt.Errorf("compile catalog-pinned KubaraConfiguration CRD: %w", err)
	}
	return validator, nil
}

// ValidateKubaraConfigurationTransition validates a proposed CR against its
// previous GitOps version and returns the normalized proposed configuration.
func ValidateKubaraConfigurationTransition(ctx context.Context, oldData, proposedData []byte) (*Config, error) {
	oldObject, err := manifest.DecodeOne(bytes.NewReader(oldData))
	if err != nil {
		return nil, fmt.Errorf("decode previous KubaraConfiguration manifest: %w", err)
	}
	proposedObject, err := manifest.DecodeOne(bytes.NewReader(proposedData))
	if err != nil {
		return nil, fmt.Errorf("decode proposed KubaraConfiguration manifest: %w", err)
	}
	validator, err := configurationValidator()
	if err != nil {
		return nil, err
	}
	result := validator.ValidateTransition(ctx, proposedObject, oldObject, crdvalidate.RejectUnknown)
	if !result.Valid() {
		return nil, fmt.Errorf(
			"validate KubaraConfiguration transition from %q to %q: %w",
			oldObject.Name(),
			proposedObject.Name(),
			result.Err(),
		)
	}
	return configFromKubaraConfiguration(result.Object)
}

func configFromKubaraConfiguration(object *manifest.Object) (*Config, error) {
	spec, ok := object.Data()["spec"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("normalized KubaraConfiguration %q has no spec object", object.Name())
	}
	cfg := &Config{Version: ConfigVersionV1Alpha4}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "json",
		WeaklyTypedInput: false,
		Result:           cfg,
		Squash:           true,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize KubaraConfiguration decoder: %w", err)
	}
	if err := decoder.Decode(spec); err != nil {
		return nil, fmt.Errorf("decode normalized KubaraConfiguration %q: %w", object.Name(), err)
	}
	normalizeDisabledTerraform(cfg)
	return cfg, nil
}
