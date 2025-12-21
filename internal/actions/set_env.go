package actions

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// SetEnvAction implements environment variable setting
type SetEnvAction struct{ BaseAction }

// IsDeterministic returns true because set_env produces identical results.
func (SetEnvAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *SetEnvAction) Name() string {
	return "set_env"
}

// Execute sets environment variables
//
// Parameters:
//   - vars (required): List of environment variables [{name: "JAVA_HOME", value: "{install_dir}"}]
func (a *SetEnvAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get vars list (required)
	varsRaw, ok := params["vars"]
	if !ok {
		return fmt.Errorf("set_env action requires 'vars' parameter")
	}

	// Parse vars list
	envVars, err := a.parseVars(varsRaw)
	if err != nil {
		return fmt.Errorf("failed to parse vars: %w", err)
	}

	// Build standard vars for substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir, ctx.LibsDir)

	fmt.Printf("   Setting %d environment variable(s)\n", len(envVars))

	// Create env file
	envFilePath := filepath.Join(ctx.InstallDir, "env.sh")
	envFile, err := os.Create(envFilePath)
	if err != nil {
		return fmt.Errorf("failed to create env file: %w", err)
	}
	defer envFile.Close()

	// Write environment variables
	for _, envVar := range envVars {
		value := ExpandVars(envVar.Value, vars)

		// Write to env file
		fmt.Fprintf(envFile, "export %s=%s\n", envVar.Name, value)

		fmt.Printf("   ✓ %s=%s\n", envVar.Name, value)
	}

	fmt.Printf("   Environment file: %s\n", envFilePath)
	return nil
}

// parseVars parses the vars parameter
func (a *SetEnvAction) parseVars(raw interface{}) ([]recipe.EnvVar, error) {
	// Handle []interface{} from TOML
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("vars must be an array")
	}

	var result []recipe.EnvVar

	for i, item := range arr {
		varMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("var %d: must be a map", i)
		}

		name, ok := varMap["name"].(string)
		if !ok {
			return nil, fmt.Errorf("var %d: 'name' must be a string", i)
		}

		value, ok := varMap["value"].(string)
		if !ok {
			return nil, fmt.Errorf("var %d: 'value' must be a string", i)
		}

		result = append(result, recipe.EnvVar{Name: name, Value: value})
	}

	return result, nil
}
