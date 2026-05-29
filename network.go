package system

import (
	"context"
	"net"
)

// Network provides primitive network access.
type Network interface {
	DialContext(context.Context, string, string) (net.Conn, error)
	Resolver() Resolver
}

// Resolver resolves DNS names through a network boundary.
type Resolver interface {
	LookupHost(context.Context, string) ([]string, error)
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
	LookupCNAME(context.Context, string) (string, error)
	LookupMX(context.Context, string) ([]*net.MX, error)
	LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error)
	LookupTXT(context.Context, string) ([]string, error)
}
