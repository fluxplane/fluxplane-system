package hostsystem_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fluxplane/fluxplane-system/hostsystem"
	"github.com/fluxplane/fluxplane-system/systemkit"
)

func TestFacadeDoHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte("response"))
	}))
	defer server.Close()

	host, err := hostsystem.New(hostsystem.Config{Root: t.TempDir(), Network: hostsystem.NetworkConfig{AllowPrivate: true}})
	if err != nil {
		t.Fatal(err)
	}
	facade := systemkit.New(host)
	resp, err := facade.DoHTTP(context.Background(), systemkit.HTTPRequest{URL: server.URL, Method: http.MethodPost, Body: []byte("body"), MaxBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "resp" || !resp.Truncated {
		t.Fatalf("body=%q truncated=%v, want resp true", resp.Body, resp.Truncated)
	}
}

func TestNetworkDialContextLocalListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.WriteString(conn, "ok")
	}()
	network := hostsystem.NewNetwork(hostsystem.NetworkConfig{AllowPrivate: true})
	conn, err := network.DialContext(context.Background(), "tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	data := make([]byte, 2)
	if _, err := io.ReadFull(conn, data); err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok" {
		t.Fatalf("data = %q", data)
	}
	<-done
}

func TestNetworkBlocksPrivateTargets(t *testing.T) {
	network := hostsystem.NewNetwork(hostsystem.NetworkConfig{})
	_, err := network.DialContext(context.Background(), "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("expected private target block")
	}
}

func TestNetworkResolverInterface(t *testing.T) {
	network := hostsystem.NewNetwork(hostsystem.NetworkConfig{})
	if network.Resolver() == nil {
		t.Fatal("resolver is nil")
	}
}
