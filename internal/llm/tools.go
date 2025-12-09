package llm

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// Tool names for the recipe generation conversation.
const (
	ToolFetchFile      = "fetch_file"
	ToolInspectArchive = "inspect_archive"
	ToolExtractPattern = "extract_pattern"
)

// FetchFileInput is the input schema for the fetch_file tool.
type FetchFileInput struct {
	URL string `json:"url"`
}

// InspectArchiveInput is the input schema for the inspect_archive tool.
type InspectArchiveInput struct {
	URL string `json:"url"`
}

// PlatformMapping represents a mapping from a release asset to a platform.
type PlatformMapping struct {
	Asset  string `json:"asset"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Format string `json:"format"`
}

// ExtractPatternInput is the input schema for the extract_pattern tool.
// When this tool is called, the conversation ends.
type ExtractPatternInput struct {
	Mappings       []PlatformMapping `json:"mappings"`
	Executable     string            `json:"executable"`
	VerifyCommand  string            `json:"verify_command"`
	StripPrefix    string            `json:"strip_prefix,omitempty"`
	InstallSubpath string            `json:"install_subpath,omitempty"`
}

// toolSchemas returns the tool definitions for the recipe generation conversation.
func toolSchemas() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name:        ToolFetchFile,
				Description: anthropic.String("Fetch a file from a URL to examine its contents. Use this to inspect README files, documentation, or other text files that might help understand the tool's installation requirements."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "The URL to fetch the file from",
						},
					},
					Required: []string{"url"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        ToolInspectArchive,
				Description: anthropic.String("Download and inspect the contents of an archive (tar.gz, zip, etc.) to see what files are inside. Use this to understand the archive structure and find the executable binary."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "The URL of the archive to inspect",
						},
					},
					Required: []string{"url"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        ToolExtractPattern,
				Description: anthropic.String("Signal that you have discovered the asset pattern. Call this when you understand how release assets map to platforms and are ready to generate the recipe. This ends the conversation."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: "object",
					Properties: map[string]interface{}{
						"mappings": map[string]interface{}{
							"type":        "array",
							"description": "List of asset-to-platform mappings",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"asset": map[string]interface{}{
										"type":        "string",
										"description": "The release asset filename",
									},
									"os": map[string]interface{}{
										"type":        "string",
										"description": "The operating system (linux, darwin, windows)",
									},
									"arch": map[string]interface{}{
										"type":        "string",
										"description": "The architecture (amd64, arm64)",
									},
									"format": map[string]interface{}{
										"type":        "string",
										"description": "The archive format (tar.gz, zip, binary)",
									},
								},
								"required": []string{"asset", "os", "arch", "format"},
							},
						},
						"executable": map[string]interface{}{
							"type":        "string",
							"description": "The name of the executable binary inside the archive",
						},
						"verify_command": map[string]interface{}{
							"type":        "string",
							"description": "Command to verify the installation works (e.g., 'gh --version')",
						},
						"strip_prefix": map[string]interface{}{
							"type":        "string",
							"description": "Optional prefix to strip from archive paths during extraction",
						},
						"install_subpath": map[string]interface{}{
							"type":        "string",
							"description": "Optional subpath within the archive where the executable is located",
						},
					},
					Required: []string{"mappings", "executable", "verify_command"},
				},
			},
		},
	}
}
