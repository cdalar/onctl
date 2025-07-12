package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestTemplate_StructBasics(t *testing.T) {
	// Test Template struct creation and field access
	template := Template{
		Name:        "test-template",
		Description: "Test description",
		Config:      "test-config.yaml",
		Type:        "test-type",
	}

	assert.Equal(t, "test-template", template.Name)
	assert.Equal(t, "Test description", template.Description)
	assert.Equal(t, "test-config.yaml", template.Config)
	assert.Equal(t, "test-type", template.Type)
}

func TestTemplateIndex_StructBasics(t *testing.T) {
	// Test TemplateIndex struct creation and field access
	template1 := Template{Name: "template1", Type: "type1"}
	template2 := Template{Name: "template2", Type: "type2"}
	
	index := TemplateIndex{
		Templates: []Template{template1, template2},
	}

	assert.Len(t, index.Templates, 2)
	assert.Equal(t, "template1", index.Templates[0].Name)
	assert.Equal(t, "template2", index.Templates[1].Name)
}

func TestTemplateIndex_YAMLUnmarshal(t *testing.T) {
	yamlContent := `templates:
  - name: basic-k8s
    description: Basic Kubernetes setup
    config: k8s-basic.yaml
    type: kubernetes
  - name: docker-compose
    description: Docker compose setup
    config: compose.yaml
    type: docker
`
	var index TemplateIndex
	err := yaml.Unmarshal([]byte(yamlContent), &index)
	assert.NoError(t, err)

	assert.Len(t, index.Templates, 2)
	assert.Equal(t, "basic-k8s", index.Templates[0].Name)
	assert.Equal(t, "Basic Kubernetes setup", index.Templates[0].Description)
	assert.Equal(t, "k8s-basic.yaml", index.Templates[0].Config)
	assert.Equal(t, "kubernetes", index.Templates[0].Type)
}

func TestTemplateIndex_YAMLMarshal(t *testing.T) {
	// Test that our structs can be marshaled to YAML
	template := Template{
		Name:        "test",
		Description: "Test template",
		Config:      "config.yaml",
		Type:        "test-type",
	}
	
	index := TemplateIndex{
		Templates: []Template{template},
	}

	yamlData, err := yaml.Marshal(index)
	assert.NoError(t, err)
	assert.Contains(t, string(yamlData), "test")
	assert.Contains(t, string(yamlData), "Test template")
}

func TestTemplateIndex_EmptyTemplates(t *testing.T) {
	// Test empty templates list
	index := TemplateIndex{
		Templates: []Template{},
	}

	assert.Len(t, index.Templates, 0)
	assert.NotNil(t, index.Templates)
}

func TestTemplate_ZeroValues(t *testing.T) {
	// Test zero value Template
	var template Template
	
	assert.Equal(t, "", template.Name)
	assert.Equal(t, "", template.Description)
	assert.Equal(t, "", template.Config)
	assert.Equal(t, "", template.Type)
}

func TestTemplatesCmd_CommandProperties(t *testing.T) {
	// Test that the command has the expected properties
	assert.Equal(t, "templates", templatesCmd.Use)
	assert.Contains(t, templatesCmd.Aliases, "tmpl")
	assert.Equal(t, "Manage onctl templates", templatesCmd.Short)
	assert.Contains(t, templatesCmd.Long, "templates.onctl.com")
}

func TestTemplatesListCmd_CommandProperties(t *testing.T) {
	// Test that the list command has the expected properties
	assert.Equal(t, "list", templatesListCmd.Use)
	assert.Contains(t, templatesListCmd.Aliases, "ls")
	assert.Equal(t, "List available templates", templatesListCmd.Short)
	assert.NotNil(t, templatesListCmd.Run)
}

func TestTemplatesCmd_HasSubCommands(t *testing.T) {
	// Test that templates command has the list subcommand
	subCommands := templatesCmd.Commands()
	found := false
	for _, cmd := range subCommands {
		if cmd.Name() == "list" {
			found = true
			break
		}
	}
	assert.True(t, found, "templates command should have 'list' subcommand")
}

func TestTemplatesListCmd_HasFlags(t *testing.T) {
	// Test that flags are properly registered
	flag := templatesListCmd.Flags().Lookup("file")
	assert.NotNil(t, flag, "templates list command should have 'file' flag")
	assert.Equal(t, "f", flag.Shorthand, "file flag should have 'f' shorthand")
	assert.Equal(t, "", flag.DefValue, "file flag should have empty default value")
	assert.Equal(t, "local index.yaml file path", flag.Usage)
}

func TestTemplate_YAMLTags(t *testing.T) {
	// Test YAML marshaling with proper tags
	template := Template{
		Name:        "yaml-test",
		Description: "YAML test template",
		Config:      "yaml-config.yaml",
		Type:        "yaml-type",
	}

	yamlData, err := yaml.Marshal(template)
	assert.NoError(t, err)
	
	yamlStr := string(yamlData)
	assert.Contains(t, yamlStr, "name: yaml-test")
	assert.Contains(t, yamlStr, "description: YAML test template")
	assert.Contains(t, yamlStr, "config: yaml-config.yaml")
	assert.Contains(t, yamlStr, "type: yaml-type")
}

func TestTemplateIndex_YAMLTags(t *testing.T) {
	// Test YAML marshaling of TemplateIndex with proper tags
	template := Template{
		Name: "index-test",
		Type: "index-type",
	}
	
	index := TemplateIndex{
		Templates: []Template{template},
	}

	yamlData, err := yaml.Marshal(index)
	assert.NoError(t, err)
	
	yamlStr := string(yamlData)
	assert.Contains(t, yamlStr, "templates:")
	assert.Contains(t, yamlStr, "name: index-test")
}
