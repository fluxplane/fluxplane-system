package hostsystem

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Environment implements environment lookup using the host process environment.
type Environment struct{}

// ProcessEnvironment optionally supplies the complete environment for process
// execution. It is used by wrappers that need stricter env policy than the
// default host environment plus overrides behavior.
type ProcessEnvironment interface {
	ProcessEnv(context.Context, []string) ([]string, error)
}

func (Environment) Lookup(_ context.Context, key string) (string, bool, error) {
	value, ok := os.LookupEnv(strings.TrimSpace(key))
	return value, ok, nil
}

func (Environment) ResolveExecutable(_ context.Context, name string) (string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsRune(name, filepath.Separator) {
		return "", false, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false, nil
	}
	return path, true, nil
}

func (Environment) ProcessEnv(_ context.Context, overrides []string) ([]string, error) {
	return processEnv(overrides)
}

func processEnv(overrides []string) ([]string, error) {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key != "" {
			values[key] = value
		}
	}
	for _, entry := range overrides {
		key, value, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("process env entry %q is invalid", entry)
		}
		if strings.ContainsAny(value, "\x00\n\r") {
			return nil, fmt.Errorf("process env value for %s contains unsupported control character", key)
		}
		values[key] = value
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out, nil
}
