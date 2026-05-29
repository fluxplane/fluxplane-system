// Package pathpattern compiles and matches slash-style relative path patterns.
package pathpattern

import (
	"fmt"
	"path"
	"strings"
)

// Pattern is a compiled slash-style relative path pattern.
type Pattern struct {
	raw      string
	patterns [][]string
}

// Compile validates and compiles pattern.
func Compile(pattern string) (Pattern, error) {
	pattern = strings.TrimSpace(path.Clean(strings.ReplaceAll(pattern, "\\", "/")))
	if pattern == "." || pattern == "" {
		return Pattern{}, fmt.Errorf("path pattern is empty")
	}
	if path.IsAbs(pattern) {
		return Pattern{}, fmt.Errorf("path pattern %q must be relative", pattern)
	}
	for _, part := range strings.Split(pattern, "/") {
		if part == ".." {
			return Pattern{}, fmt.Errorf("path pattern %q escapes its root", pattern)
		}
	}
	expanded, err := expandBraces(pattern)
	if err != nil {
		return Pattern{}, err
	}
	compiled := Pattern{raw: pattern, patterns: make([][]string, 0, len(expanded))}
	for _, candidate := range expanded {
		segments := strings.Split(candidate, "/")
		for _, segment := range segments {
			if segment == "**" {
				continue
			}
			if _, err := path.Match(segment, ""); err != nil {
				return Pattern{}, fmt.Errorf("path pattern %q: %w", pattern, err)
			}
		}
		compiled.patterns = append(compiled.patterns, segments)
	}
	return compiled, nil
}

// Match reports whether rel matches p.
func (p Pattern) Match(rel string) bool {
	rel = strings.TrimSpace(path.Clean(strings.ReplaceAll(rel, "\\", "/")))
	if rel == "." || rel == "" || path.IsAbs(rel) {
		return false
	}
	segments := strings.Split(rel, "/")
	for _, pattern := range p.patterns {
		if matchSegments(pattern, segments) {
			return true
		}
	}
	return false
}

func matchSegments(pattern, rel []string) bool {
	if len(pattern) == 0 {
		return len(rel) == 0
	}
	if pattern[0] == "**" {
		if matchSegments(pattern[1:], rel) {
			return true
		}
		for i := range rel {
			if matchSegments(pattern[1:], rel[i+1:]) {
				return true
			}
		}
		return false
	}
	if len(rel) == 0 {
		return false
	}
	ok, err := path.Match(pattern[0], rel[0])
	return err == nil && ok && matchSegments(pattern[1:], rel[1:])
}

func expandBraces(pattern string) ([]string, error) {
	start := strings.IndexByte(pattern, '{')
	if start == -1 {
		if strings.Contains(pattern, "}") {
			return nil, fmt.Errorf("path pattern %q has unmatched brace", pattern)
		}
		return []string{pattern}, nil
	}
	end := strings.IndexByte(pattern[start+1:], '}')
	if end == -1 {
		return nil, fmt.Errorf("path pattern %q has unmatched brace", pattern)
	}
	end += start + 1
	body := pattern[start+1 : end]
	if body == "" || strings.ContainsAny(body, "{}") {
		return nil, fmt.Errorf("path pattern %q has malformed brace alternation", pattern)
	}
	alts := strings.Split(body, ",")
	out := make([]string, 0, len(alts))
	for _, alt := range alts {
		if alt == "" {
			return nil, fmt.Errorf("path pattern %q has empty brace alternative", pattern)
		}
		out = append(out, pattern[:start]+alt+pattern[end+1:])
	}
	if strings.ContainsAny(pattern[end+1:], "{}") {
		return nil, fmt.Errorf("path pattern %q has unsupported nested or multiple brace alternation", pattern)
	}
	return out, nil
}
