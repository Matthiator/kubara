package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"go.yaml.in/yaml/v3"
)

func applyLegacyDefaults(raw map[string]any) {
	clusters, _ := raw["clusters"].([]any)
	for _, value := range clusters {
		cluster, ok := value.(map[string]any)
		if !ok {
			continue
		}
		setDefault(cluster, "stage", "dev")
		setDefault(cluster, "type", Hub)
		setDefault(cluster, "ingressClassName", "traefik")

		if terraform, ok := cluster["terraform"].(map[string]any); ok {
			setDefault(terraform, "provider", string(TerraformProviderNone))
			setDefault(terraform, "kubernetesType", "ske")
		}
		argocd, ok := cluster["argocd"].(map[string]any)
		if !ok {
			continue
		}
		setDefault(argocd, "selfManaged", string(ArgoCDSelfManagedEnabled))
		repo, ok := argocd["repo"].(map[string]any)
		if !ok {
			continue
		}
		for _, protocol := range []string{"https", "oci"} {
			repositoryType, ok := repo[protocol].(map[string]any)
			if !ok {
				continue
			}
			for _, name := range []string{"configs", "components"} {
				repository, ok := repositoryType[name].(map[string]any)
				if ok {
					setDefault(repository, "path", "")
					setDefault(repository, "targetRevision", "main")
				}
			}
		}
	}
}

func setDefault(object map[string]any, key string, value any) {
	current, exists := object[key]
	if !exists || current == nil || current == "" {
		object[key] = value
	}
}

func kubaraConfigurationDocument(cfg *Config, name string) ([]byte, error) {
	if name == "" {
		name = "platform"
	}

	spec, err := configSpec(cfg)
	if err != nil {
		return nil, err
	}
	document := map[string]any{
		"apiVersion": KubaraConfigurationAPIVersion,
		"kind":       KubaraConfigurationKind,
		"metadata":   map[string]any{"name": name},
		"spec":       spec,
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, fmt.Errorf("marshal KubaraConfiguration: %w", err)
	}
	out := output.Bytes()
	return out, nil
}

func configSpec(cfg *Config) (map[string]any, error) {
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal normalized config: %w", err)
	}
	var spec map[string]any
	if err := json.Unmarshal(encoded, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal normalized config: %w", err)
	}
	delete(spec, "version")
	return spec, nil
}
