package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/wagiedev/codex-agent-sdk-go/internal/config"
)

// Command represents the CLI command to execute.
type Command struct {
	// Args are the command line arguments.
	Args []string

	// Env are the environment variables.
	Env []string
}

// BuildExecArgs constructs the CLI argument list for a one-shot `codex exec` invocation.
func BuildExecArgs(prompt string, options *config.Options) []string {
	args := []string{
		"exec",
		"--json",
		"--full-auto",
		"--ephemeral",
		"--skip-git-repo-check",
	}

	if options.Model != "" {
		args = append(args, "-m", options.Model)
	}

	// Sandbox mode: explicit Sandbox field takes precedence, then map from PermissionMode
	sandbox := options.Sandbox
	if sandbox == "" {
		sandbox = mapPermissionToSandbox(options.PermissionMode)
	}

	if sandbox != "" {
		args = append(args, "-s", sandbox)
	}

	cwd := options.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	if cwd != "" {
		args = append(args, "-C", cwd)
	}

	for _, dir := range options.AddDirs {
		args = append(args, "--add-dir", dir)
	}

	for _, img := range options.Images {
		args = append(args, "-i", img)
	}

	// Sort config keys for deterministic flag ordering.
	if len(options.Config) > 0 {
		keys := make([]string, 0, len(options.Config))
		for k := range options.Config {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			args = append(args, "-c", fmt.Sprintf("%s=%s", k, options.Config[k]))
		}
	}

	if options.OutputSchema != "" {
		args = append(args, "--output-schema", options.OutputSchema)
	}

	// Extra args from the ExtraArgs map
	for key, value := range options.ExtraArgs {
		if value == nil {
			args = append(args, "--"+key)
		} else {
			args = append(args, "--"+key, *value)
		}
	}

	// Prompt is always the last positional argument.
	args = append(args, prompt)

	return args
}

// BuildAppServerArgs constructs the CLI argument list for `codex app-server`.
func BuildAppServerArgs(_ *config.Options) []string {
	return []string{"app-server"}
}

// BuildEnvironment constructs the environment variables for the CLI process.
func BuildEnvironment(options *config.Options) []string {
	env := os.Environ()
	env = append(env, "CODEX_CLI_SDK_VERSION=0.1.0")

	for key, value := range options.Env {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// mapPermissionToSandbox maps claude-sdk permission modes to Codex sandbox modes.
func mapPermissionToSandbox(permMode string) string {
	switch permMode {
	case "default", "":
		return ""
	case "acceptEdits":
		return "workspace-write"
	case "bypassPermissions", "acceptAll":
		return "danger-full-access"
	default:
		return ""
	}
}
