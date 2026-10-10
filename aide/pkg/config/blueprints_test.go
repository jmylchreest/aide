package config

import (
	"reflect"
	"testing"
)

func TestBlueprintRegistryLayers(t *testing.T) {
	isolateHome(t)
	prior := Get()
	t.Cleanup(func() { Set(prior) })
	root := t.TempDir()
	global := []string{"https://global.example/first", "https://global.example/second"}
	writeGlobalConfig(t, `{"blueprints":{"registries":["https://global.example/first","https://global.example/second"]}}`)
	for _, tc := range []struct {
		name, project string
		want          []string
	}{
		{"inherit", `{}`, global},
		{"replace", `{"blueprints":{"registries":["https://project.example"]}}`, []string{"https://project.example"}},
		{"disable", `{"blueprints":{"registries":[]}}`, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeProjectConfig(t, root, tc.project)
			cfg, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.Blueprints.Registries, tc.want) {
				t.Fatalf("registries=%v want=%v", cfg.Blueprints.Registries, tc.want)
			}
		})
	}
}
