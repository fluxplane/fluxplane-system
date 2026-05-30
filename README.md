# fluxplane-system

Primitive host capability contracts and implementations for Fluxplane modules.

This module defines the low-level system boundary used by Fluxplane runtimes and plugins. It is intentionally policy-neutral: callers decide what is safe, while this package exposes portable interfaces and small adapters for filesystem, process, environment, network, path, and host information capabilities.

## Usage

```go
import system "github.com/fluxplane/fluxplane-system"

func needsWorkspace(fs system.FileSystem) error {
    entries, err := fs.ReadDir(".")
    if err != nil {
        return err
    }
    _ = entries
    return nil
}
```

## Packages

- root package: shared capability interfaces and helper functions.
- `hostsystem`: concrete host-backed system implementation.
- `memsystem`: in-memory system implementation for tests and sandboxed callers.
- `mountfs`: mounted filesystem composition helpers.
- `systemkit`: reusable wiring helpers for system implementations.
- `systemtest`: test utilities for system implementations.

## Design

`fluxplane-system` stays below Fluxplane runtime policy. It should not know about workspaces, plugins, auth, endpoints, datasources, or operation safety. Higher-level modules wrap these primitives with authorization and product-specific behavior.
