package hostsystem

import (
	"context"
	"net"
	"time"

	"github.com/fluxplane/fluxplane-system"
)

// Network implements primitive network access using net.Dialer and net.Resolver.
type Network struct {
	dialer   *net.Dialer
	resolver *Resolver
}

func NewNetwork() *Network {
	return &Network{dialer: &net.Dialer{Timeout: 30 * time.Second}, resolver: &Resolver{}}
}

func (n *Network) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := n.dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 30 * time.Second}
	}
	return dialer.DialContext(ctx, network, address)
}

func (n *Network) Resolver() system.Resolver {
	if n == nil || n.resolver == nil {
		return Resolver{}
	}
	return *n.resolver
}

// Resolver resolves DNS names using net.DefaultResolver.
type Resolver struct{}

func (Resolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

func (Resolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

func (Resolver) LookupCNAME(ctx context.Context, host string) (string, error) {
	return net.DefaultResolver.LookupCNAME(ctx, host)
}

func (Resolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	return net.DefaultResolver.LookupMX(ctx, name)
}

func (Resolver) LookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error) {
	return net.DefaultResolver.LookupSRV(ctx, service, proto, name)
}

func (Resolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return net.DefaultResolver.LookupTXT(ctx, name)
}
