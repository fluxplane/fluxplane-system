package system

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// EnvFileSet contains env values loaded from env files.
type EnvFileSet struct {
	Files  []string
	Values map[string]string
}

// LoadEnvFiles resolves and parses env files relative to root. Later files override earlier values.
func LoadEnvFiles(root string, patterns []string) (EnvFileSet, error) {
	files, err := ResolveEnvFiles(root, patterns)
	if err != nil {
		return EnvFileSet{}, err
	}
	values := map[string]string{}
	for _, file := range files {
		loaded, err := ParseEnvFile(file)
		if err != nil {
			return EnvFileSet{}, err
		}
		for key, value := range loaded {
			values[key] = value
		}
	}
	return EnvFileSet{Files: files, Values: values}, nil
}

// ResolveExecutableInPath resolves an executable name using an explicit PATH value.
func ResolveExecutableInPath(name, pathValue string) (string, bool, error) {
	if strings.TrimSpace(name) == "" || strings.ContainsRune(name, filepath.Separator) {
		return "", false, nil
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode().Perm()&0111 == 0 {
			continue
		}
		return candidate, true, nil
	}
	return "", false, nil
}

// ParseEnvFile parses KEY=VALUE entries from path.
func ParseEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("env file %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(strings.TrimSuffix(scanner.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, rawValue, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !ValidEnvKey(key) {
			return nil, fmt.Errorf("env file %q line %d has invalid key", path, lineNo)
		}
		value, err := ParseEnvValue(strings.TrimSpace(rawValue))
		if err != nil {
			return nil, fmt.Errorf("env file %q line %d: %w", path, lineNo, err)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("env file %q: %w", path, err)
	}
	return values, nil
}

// ParseEnvValue parses one env-file value.
func ParseEnvValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	switch raw[0] {
	case '\'':
		if len(raw) < 2 || raw[len(raw)-1] != '\'' {
			return "", fmt.Errorf("unterminated single-quoted value")
		}
		return raw[1 : len(raw)-1], nil
	case '"':
		if len(raw) < 2 || raw[len(raw)-1] != '"' {
			return "", fmt.Errorf("unterminated double-quoted value")
		}
		return UnescapeDoubleQuotedEnv(raw[1 : len(raw)-1])
	default:
		return strings.TrimSpace(stripEnvComment(raw)), nil
	}
}

// UnescapeDoubleQuotedEnv unescapes supported double-quoted env value escapes.
func UnescapeDoubleQuotedEnv(value string) (string, error) {
	var out strings.Builder
	escaped := false
	for _, r := range value {
		if escaped {
			switch r {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '\\', '"':
				out.WriteRune(r)
			default:
				out.WriteByte('\\')
				out.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		out.WriteRune(r)
	}
	if escaped {
		return "", fmt.Errorf("unterminated escape")
	}
	return out.String(), nil
}

// ValidEnvKey reports whether key is a shell-style env variable name.
func ValidEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// FormatEnv formats env values as sorted KEY=VALUE entries.
func FormatEnv(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

// CloneStringMap returns a shallow clone of input.
func CloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

// TrimStrings trims strings and removes empty values.
func TrimStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, 0, len(input))
	for _, value := range input {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

// ResolveEnvFiles resolves configured env file patterns under root.
func ResolveEnvFiles(root string, patterns []string) ([]string, error) {
	var files []string
	for _, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		if pattern == "" {
			continue
		}
		absPattern, err := EnvFilePattern(root, pattern)
		if err != nil {
			return nil, err
		}
		if !hasGlobMeta(absPattern) {
			if resolved, ok, err := resolveEnvFile(root, absPattern); err != nil {
				return nil, err
			} else if ok {
				files = append(files, resolved)
			}
			continue
		}
		matches, err := filepath.Glob(absPattern)
		if err != nil {
			return nil, fmt.Errorf("env file glob %q: %w", pattern, err)
		}
		sort.Strings(matches)
		for _, match := range matches {
			resolved, ok, err := resolveEnvFile(root, match)
			if err != nil {
				return nil, err
			}
			if ok {
				files = append(files, resolved)
			}
		}
	}
	return files, nil
}

// EnvFilePattern returns the absolute pattern for an env file pattern under root.
func EnvFilePattern(root, pattern string) (string, error) {
	if filepath.IsAbs(pattern) {
		dir := StaticPatternDir(pattern)
		if err := PathWithin(root, dir); err != nil {
			return "", fmt.Errorf("env file %q escapes root", pattern)
		}
		return filepath.Clean(pattern), nil
	}
	clean := filepath.Clean(filepath.FromSlash(pattern))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("env file %q escapes root", pattern)
	}
	return filepath.Join(root, clean), nil
}

// PathWithin verifies candidate is within root.
func PathWithin(root, candidate string) error {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path %q escapes root %q", candidate, root)
	}
	return nil
}

// StaticPatternDir returns the static directory prefix for a glob pattern.
func StaticPatternDir(pattern string) string {
	idx := strings.IndexAny(pattern, "*?[")
	if idx < 0 {
		return filepath.Dir(filepath.Clean(pattern))
	}
	prefix := pattern[:idx]
	dir := filepath.Dir(prefix)
	if dir == "." || dir == "" {
		dir = string(os.PathSeparator)
	}
	return filepath.Clean(dir)
}

func resolveEnvFile(root, path string) (string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("env file %q: %w", path, err)
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("env file %q is a directory", path)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false, fmt.Errorf("env file %q: %w", path, err)
	}
	if err := PathWithin(root, real); err != nil {
		return "", false, fmt.Errorf("env file %q escapes root", path)
	}
	return real, true, nil
}

func stripEnvComment(value string) string {
	for i, r := range value {
		if r == '#' && (i == 0 || unicode.IsSpace(rune(value[i-1]))) {
			return strings.TrimSpace(value[:i])
		}
	}
	return value
}

func hasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}
