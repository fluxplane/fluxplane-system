// Package hostinfo collects read-only host metadata from a system.
package hostinfo

import (
	"context"
	"net"
	"os"
	"os/user"
	"runtime"
	"sort"
	"time"

	"github.com/fluxplane/fluxplane-system"
)

type Request struct {
	Categories []Category `json:"categories,omitempty"`
}

type Category string

const (
	CategoryOS      Category = "os"
	CategoryRuntime Category = "runtime"
	CategoryUser    Category = "user"
	CategoryPaths   Category = "paths"
	CategoryCPU     Category = "cpu"
	CategoryTime    Category = "time"
	CategoryEnv     Category = "env"
	CategoryNetwork Category = "network"
)

type Result struct {
	GeneratedAt time.Time      `json:"generated_at"`
	OS          map[string]any `json:"os,omitempty"`
	Runtime     map[string]any `json:"runtime,omitempty"`
	User        map[string]any `json:"user,omitempty"`
	Paths       map[string]any `json:"paths,omitempty"`
	CPU         map[string]any `json:"cpu,omitempty"`
	Time        map[string]any `json:"time,omitempty"`
	Env         map[string]any `json:"env,omitempty"`
	Network     map[string]any `json:"network,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
}

func Collect(ctx context.Context, sys system.System, req Request) (Result, error) {
	now := time.Now().UTC()
	if sys != nil && sys.Clock() != nil {
		now = sys.Clock().Now().UTC()
	}
	out := Result{GeneratedAt: now}
	categories := req.Categories
	if len(categories) == 0 {
		categories = []Category{CategoryOS, CategoryRuntime, CategoryUser, CategoryPaths, CategoryCPU, CategoryTime, CategoryEnv, CategoryNetwork}
	}
	for _, category := range categories {
		switch category {
		case CategoryOS:
			out.OS = collectOS()
		case CategoryRuntime:
			out.Runtime = collectRuntime()
		case CategoryUser:
			out.User = collectUser()
		case CategoryPaths:
			out.Paths = collectPaths()
		case CategoryCPU:
			out.CPU = map[string]any{"logical_cpus": runtime.NumCPU(), "gomaxprocs": runtime.GOMAXPROCS(0)}
		case CategoryTime:
			out.Time = collectTime(now)
		case CategoryEnv:
			out.Env = collectEnv(ctx, sys)
		case CategoryNetwork:
			out.Network = collectNetwork()
		}
	}
	return out, nil
}

func collectOS() map[string]any {
	out := map[string]any{"goos": runtime.GOOS, "goarch": runtime.GOARCH}
	if hostname, err := os.Hostname(); err == nil {
		out["hostname"] = hostname
	}
	return out
}

func collectRuntime() map[string]any {
	out := map[string]any{"go_version": runtime.Version(), "compiler": runtime.Compiler, "process_id": os.Getpid(), "parent_id": os.Getppid()}
	if executable, err := os.Executable(); err == nil {
		out["executable"] = executable
	}
	return out
}

func collectUser() map[string]any {
	out := map[string]any{}
	if current, err := user.Current(); err == nil {
		out["username"] = current.Username
		out["name"] = current.Name
		out["uid"] = current.Uid
		out["gid"] = current.Gid
		out["home_dir"] = current.HomeDir
	}
	return out
}

func collectPaths() map[string]any {
	out := map[string]any{"temp_dir": os.TempDir()}
	if wd, err := os.Getwd(); err == nil {
		out["working_dir"] = wd
	}
	if home, err := os.UserHomeDir(); err == nil {
		out["home_dir"] = home
	}
	if executable, err := os.Executable(); err == nil {
		out["executable"] = executable
	}
	return out
}

func collectTime(now time.Time) map[string]any {
	name, offset := now.Zone()
	return map[string]any{
		"utc": now.UTC().Format(time.RFC3339), "local": now.Format(time.RFC3339),
		"unix": now.Unix(), "timezone": time.Local.String(), "zone_name": name, "offset_seconds": offset,
	}
}

func collectEnv(ctx context.Context, sys system.System) map[string]any {
	keys := []string{"PATH", "HOME", "USER", "TMPDIR", "TEMP", "TMP", "GOCACHE", "GOPATH", "GOMODCACHE", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "DEX_HOME"}
	values := map[string]string{}
	if sys != nil && sys.Environment() != nil {
		for _, key := range keys {
			if value, ok, err := sys.Environment().Lookup(ctx, key); err == nil && ok {
				values[key] = value
			}
		}
	}
	return map[string]any{"values": values}
}

func collectNetwork() map[string]any {
	out := map[string]any{}
	interfaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	items := make([]map[string]any, 0, len(interfaces))
	for _, iface := range interfaces {
		item := map[string]any{"name": iface.Name, "index": iface.Index, "mtu": iface.MTU, "flags": iface.Flags.String(), "hardware_addr": iface.HardwareAddr.String()}
		addrs, err := iface.Addrs()
		if err == nil {
			values := make([]string, 0, len(addrs))
			for _, addr := range addrs {
				values = append(values, addr.String())
			}
			sort.Strings(values)
			item["addrs"] = values
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		left, _ := items[i]["name"].(string)
		right, _ := items[j]["name"].(string)
		return left < right
	})
	out["interfaces"] = items
	return out
}
