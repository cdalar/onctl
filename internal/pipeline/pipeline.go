package pipeline

import (
	"fmt"
	"log"
	"os"

	"github.com/cdalar/onctl/internal/cloud"
	"gopkg.in/yaml.v2"
)

// Configuration structures for pipeline orchestration
type PipelineConfig struct {
	Targets []Target `yaml:"targets"`
	Steps   []Step   `yaml:"steps"`
}

type Target struct {
	Name   string       `yaml:"name"`
	Config TargetConfig `yaml:"config"`
	// Runtime state (not in YAML) - managed externally
}

type TargetConfig struct {
	PublicKeyFile string   `yaml:"publicKeyFile"`
	Vm            cloud.Vm `yaml:"vm"`
}

type Step struct {
	Name      string     `yaml:"name"`
	Type      string     `yaml:"type"`
	Target    string     `yaml:"target"`
	DependsOn []string   `yaml:"depends_on,omitempty"`
	Config    StepConfig `yaml:"config,omitempty"`
}

type StepConfig struct {
	// For upload step
	Files []string `yaml:"files,omitempty"`

	// For apply step
	DotEnvFile string   `yaml:"dotEnvFile,omitempty"`
	Variables  []string `yaml:"variables,omitempty"`

	// Add other step-specific config fields as needed
}

// ExecConfig holds external dependencies for execution
type ExecConfig struct {
	Provider interface{} // Cloud provider interface
}

// Parse pipeline configuration from YAML file
func parsePipelineConfig(configFile string) (*PipelineConfig, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open pipeline config file %q: %w", configFile, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close pipeline config file: %v", err)
		}
	}()

	var config PipelineConfig
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline config file %q: %w", configFile, err)
	}

	return &config, nil
}

// LoadConfig loads and validates a pipeline configuration
func LoadConfig(configFile string) (*PipelineConfig, error) {
	config, err := parsePipelineConfig(configFile)
	if err != nil {
		return nil, err
	}

	// Validate configuration
	if err := validatePipelineConfig(config); err != nil {
		return nil, fmt.Errorf("invalid pipeline configuration: %w", err)
	}

	return config, nil
}

// Resolve step dependencies using topological sort
func ResolveDependencies(steps []Step) ([]Step, error) {
	// Build dependency graph
	graph := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, step := range steps {
		if _, exists := graph[step.Name]; !exists {
			graph[step.Name] = []string{}
		}
		inDegree[step.Name] = len(step.DependsOn)

		for _, dep := range step.DependsOn {
			graph[dep] = append(graph[dep], step.Name)
		}
	}

	// Kahn's algorithm for topological sort
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	var result []Step
	stepMap := make(map[string]Step)
	for _, step := range steps {
		stepMap[step.Name] = step
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if step, exists := stepMap[current]; exists {
			result = append(result, step)
		}

		for _, neighbor := range graph[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(result) != len(steps) {
		return nil, fmt.Errorf("circular dependency detected in pipeline steps")
	}

	return result, nil
}

// validatePipelineConfig checks the configuration for errors
func validatePipelineConfig(config *PipelineConfig) error {
	// Check that targets have unique names
	targetNames := make(map[string]bool)
	for _, target := range config.Targets {
		if targetNames[target.Name] {
			return fmt.Errorf("duplicate target name: %s", target.Name)
		}
		targetNames[target.Name] = true
	}

	// Check that steps reference valid targets and have unique names
	stepNames := make(map[string]bool)
	for _, step := range config.Steps {
		if stepNames[step.Name] {
			return fmt.Errorf("duplicate step name: %s", step.Name)
		}
		stepNames[step.Name] = true

		// Check target exists
		found := false
		for _, target := range config.Targets {
			if target.Name == step.Target {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("step %s references unknown target: %s", step.Name, step.Target)
		}

		// Validate step type
		switch step.Type {
		case "create", "upload", "apply", "download":
			// Valid types
		default:
			return fmt.Errorf("step %s has unknown type: %s", step.Name, step.Type)
		}

		// Check dependencies reference valid steps
		for _, dep := range step.DependsOn {
			if !stepNames[dep] {
				// Note: forward dependencies are allowed (steps can depend on future steps)
				found := false
				for _, s := range config.Steps {
					if s.Name == dep {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("step %s depends on unknown step: %s", step.Name, dep)
				}
			}
		}
	}

	return nil
}

// Executable is an interface for step executors
type Executable interface {
	ExecuteStep(step Step, target Target) error
}
