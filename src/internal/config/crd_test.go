package config

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kubara-io/kubara/internal/catalog"

	"github.com/kubara-io/libkubara/crdvalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestKubaraConfigurationCRDCompilesWithSelectedKubernetesVersion(t *testing.T) {
	definition, err := crdvalidate.DecodeCRD(bytes.NewReader(ConfigurationCRD()))
	require.NoError(t, err)
	_, err = crdvalidate.Compile(definition)
	require.NoError(t, err)
}

func TestConfigurationJSONSchemaIdentifiesTheCustomResource(t *testing.T) {
	schema, err := ConfigurationJSONSchema()
	require.NoError(t, err)
	assert.Equal(t, KubaraConfigurationKind, schema["title"])
	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, properties, "apiVersion")
	assert.Contains(t, properties, "kind")
	assert.Contains(t, properties, "metadata")
}

func TestConfigStoreLoadsNormalizedKubaraConfiguration(t *testing.T) {
	legacy := createLoadedConfigStore(t, newValidTestConfig())
	spec, err := configSpec(legacy.GetConfig())
	require.NoError(t, err)
	document := map[string]any{
		"apiVersion": KubaraConfigurationAPIVersion,
		"kind":       KubaraConfigurationKind,
		"metadata":   map[string]any{"name": "platform"},
		"spec":       spec,
	}
	data, err := yaml.Marshal(document)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "kubara-configuration.yaml")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	store := NewConfigStore(filepath.Dir(path), path, catalog.LoadOptions{})
	require.NoError(t, store.Load())
	assert.Equal(t, legacy.GetConfig(), store.GetConfig())
}

func TestKubaraConfigurationRejectsUnknownFields(t *testing.T) {
	document := []byte(`apiVersion: kubara.io/v1alpha5
kind: KubaraConfiguration
metadata:
  name: platform
spec:
  clusters: []
  unsupported: true
`)
	_, err := decodeKubaraConfiguration(document)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestValidateKubaraConfigurationTransition(t *testing.T) {
	old := []byte(`apiVersion: kubara.io/v1alpha5
kind: KubaraConfiguration
metadata:
  name: platform
spec:
  clusters: []
`)
	proposed := []byte(`apiVersion: kubara.io/v1alpha5
kind: KubaraConfiguration
metadata:
  name: platform
spec:
  bootstrapCatalog: oci://example.invalid/catalog:v1
  clusters: []
`)
	cfg, err := ValidateKubaraConfigurationTransition(context.Background(), old, proposed)
	require.NoError(t, err)
	assert.Equal(t, "oci://example.invalid/catalog:v1", *cfg.BootstrapCatalog)

	_, err = ValidateKubaraConfigurationTransition(context.Background(), old, bytes.Replace(proposed, []byte("name: platform"), []byte("name: other"), 1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transition")
}
