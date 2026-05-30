package system

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RelPathEscapes reports whether rel is outside its root.
func RelPathEscapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel)
}

// CommonPathAncestor returns the deepest common filesystem ancestor of a and b.
func CommonPathAncestor(a, b string) (string, error) {
	volumeA := filepath.VolumeName(a)
	volumeB := filepath.VolumeName(b)
	if !strings.EqualFold(volumeA, volumeB) {
		return "", fmt.Errorf("paths must share a filesystem volume")
	}
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	rel, err := filepath.Rel(a, b)
	if err == nil && !RelPathEscapes(rel) {
		return a, nil
	}
	for {
		parent := filepath.Dir(a)
		if parent == a {
			if volumeA != "" {
				return volumeA + string(os.PathSeparator), nil
			}
			return parent, nil
		}
		a = parent
		rel, err = filepath.Rel(a, b)
		if err == nil && !RelPathEscapes(rel) {
			return a, nil
		}
	}
}

// ResolveExistingUnderRoot resolves an existing path and rejects symlink escapes from root.
func ResolveExistingUnderRoot(root, candidate string) (string, error) {
	real, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if err := PathWithin(realRootOrSelf(root), real); err != nil {
		return "", err
	}
	return real, nil
}

// ResolveCreateUnderRoot resolves a path whose final component may not exist and rejects symlink escapes from root.
func ResolveCreateUnderRoot(root, candidate string) (string, error) {
	root = realRootOrSelf(root)
	if _, err := os.Lstat(candidate); err == nil {
		real, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", err
		}
		if err := PathWithin(root, real); err != nil {
			return "", err
		}
		return real, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	missing := []string{filepath.Base(candidate)}
	parent := filepath.Dir(candidate)
	for {
		if _, err := os.Lstat(parent); err == nil {
			realParent, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				realParent = filepath.Join(realParent, missing[i])
			}
			if err := PathWithin(root, realParent); err != nil {
				return "", err
			}
			return realParent, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", fmt.Errorf("path escapes root")
		}
		missing = append(missing, filepath.Base(parent))
		parent = next
	}
}

func realRootOrSelf(root string) string {
	if real, err := filepath.EvalSymlinks(root); err == nil {
		return real
	}
	return root
}
