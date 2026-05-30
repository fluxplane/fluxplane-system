package hostsystem

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/fluxplane/fluxplane-system"
)

const defaultNetworkDialTimeout = 30 * time.Second

// NetworkConfig configures a local host-backed network boundary.
type NetworkConfig struct {
	DialTimeout  time.Duration
	AllowPrivate bool
}

// Network implements primitive network access using net.Dialer and net.Resolver.
type Network struct {
	dialer       *net.Dialer
	resolver     *Resolver
	allowPrivate bool
}

func NewNetwork(cfg NetworkConfig) *Network {
	timeout := cfg.DialTimeout
	if timeout <= 0 {
		timeout = defaultNetworkDialTimeout
	}
	return &Network{dialer: &net.Dialer{Timeout: timeout}, resolver: &Resolver{}, allowPrivate: cfg.AllowPrivate}
}

func (n *Network) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	allowPrivate := n != nil && n.allowPrivate
	ip, err := ResolveAllowedIP(ctx, host, allowPrivate)
	if err != nil {
		return nil, err
	}
	dialer := (*net.Dialer)(nil)
	if n != nil {
		dialer = n.dialer
	}
	if dialer == nil {
		dialer = &net.Dialer{Timeout: defaultNetworkDialTimeout}
	}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
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

// ValidateHTTPURL rejects non-HTTP and disallowed private/local literal-IP targets.
func ValidateHTTPURL(parsed *url.URL, allowPrivate bool) error {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("url must be absolute http or https")
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("url host is empty")
	}
	if ip := net.ParseIP(host); ip != nil && !allowPrivate && BlockedIP(ip) {
		return fmt.Errorf("private, local, multicast, and metadata network targets are blocked")
	}
	return nil
}

// PublicTransport returns a guarded HTTP transport.
func PublicTransport(allowPrivate bool) http.RoundTripper {
	return PublicTransportWithTLS(allowPrivate, nil)
}

// PublicTransportWithTLS returns a guarded HTTP transport with optional
// caller-provided TLS settings.
func PublicTransportWithTLS(allowPrivate bool, cfg *tls.Config) http.RoundTripper {
	network := NewNetwork(NetworkConfig{AllowPrivate: allowPrivate, DialTimeout: 10 * time.Second})
	return &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DialContext:       network.DialContext,
		TLSClientConfig:   SecureTLSConfig(cfg),
		ForceAttemptHTTP2: true,
	}
}

// SecureTLSConfig returns cfg with at least TLS 1.2 enforced.
func SecureTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}
	out := cfg.Clone()
	if out.MinVersion == 0 || out.MinVersion < tls.VersionTLS12 {
		out.MinVersion = tls.VersionTLS12
	}
	return out
}

// ResolveAllowedIP resolves host to an IP accepted by the allowPrivate policy.
func ResolveAllowedIP(ctx context.Context, host string, allowPrivate bool) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !allowPrivate && BlockedIP(ip) {
			return nil, fmt.Errorf("private, local, multicast, and metadata network targets are blocked")
		}
		return ip, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if allowPrivate || !BlockedIP(addr.IP) {
			return addr.IP, nil
		}
	}
	return nil, fmt.Errorf("host resolves only to private, local, multicast, or metadata addresses")
}

// BlockedIP reports whether ip is unsafe for public-only network access.
func BlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() ||
		ip.Equal(net.ParseIP("169.254.169.254"))
}

var _ system.Network = (*Network)(nil)
var _ system.Resolver = Resolver{}
