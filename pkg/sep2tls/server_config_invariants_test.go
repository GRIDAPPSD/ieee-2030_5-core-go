package sep2tls_test

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2cert"
	sepTLS "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls"
	gotls "github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/sep2tls/gotls"
)

// serverInvariantMaterials mints one CA and server cert, as PEM bytes and as
// temp files, so every exported server constructor in this package can be
// built from the same identity: some take file paths, some take PEM bytes.
type serverInvariantMaterials struct {
	certPEM, keyPEM, caPEM    []byte
	certPath, keyPath, caPath string
}

func newServerInvariantMaterials(t *testing.T) serverInvariantMaterials {
	t.Helper()

	caCertPEM, caKeyPEM, err := sep2cert.GenerateCA(sep2cert.CAOptions{
		CommonName: "Server Invariant Test CA",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	caCert, caKey := parseCACert(t, caCertPEM, caKeyPEM)

	serverCertPEM, serverKeyPEM, err := sep2cert.GenerateServerCert(caCert, caKey, sep2cert.ServerCertOptions{
		Hosts:      []string{"127.0.0.1", "localhost"},
		CommonName: "Server Invariant Test Server",
		ValidYears: 1,
	})
	if err != nil {
		t.Fatalf("GenerateServerCert: %v", err)
	}

	dir := t.TempDir()
	write := func(name string, content []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, content, 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
		return p
	}

	return serverInvariantMaterials{
		certPEM:  serverCertPEM,
		keyPEM:   serverKeyPEM,
		caPEM:    caCertPEM,
		certPath: write("server.pem", serverCertPEM),
		keyPath:  write("server-key.pem", serverKeyPEM),
		caPath:   write("ca.pem", caCertPEM),
	}
}

// TestServerConfigSecurityInvariants locks three properties across every
// exported server TLS config constructor in this package (core-go #143
// review findings H1 and H2), so a future edit to any one of them cannot
// silently regress them: the TLS 1.2 cap, a client certificate requirement
// paired with a non-nil verify callback (RequireAnyClientCert with a nil
// VerifyPeerCertificate accepts any cert from any CA, an unverified-client
// auth bypass), and session tickets disabled (a resumed session skips
// VerifyPeerCertificate, bypassing the HardwareModuleName SAN check).
func TestServerConfigSecurityInvariants(t *testing.T) {
	m := newServerInvariantMaterials(t)

	t.Run("standard-library constructors", func(t *testing.T) {
		tests := []struct {
			name string
			cfg  func(t *testing.T) *tls.Config
		}{
			{"NewServerTLSConfig", func(t *testing.T) *tls.Config {
				cfg, err := sepTLS.NewServerTLSConfig(m.certPath, m.keyPath, m.caPath)
				if err != nil {
					t.Fatalf("NewServerTLSConfig: %v", err)
				}
				return cfg
			}},
			{"NewServerTLSConfigWithExtraCAs", func(t *testing.T) *tls.Config {
				cfg, err := sepTLS.NewServerTLSConfigWithExtraCAs(m.certPath, m.keyPath, m.caPath, nil)
				if err != nil {
					t.Fatalf("NewServerTLSConfigWithExtraCAs: %v", err)
				}
				return cfg
			}},
			{"NewServerTLSConfigFromPEM", func(t *testing.T) *tls.Config {
				cfg, err := sepTLS.NewServerTLSConfigFromPEM(m.certPEM, m.keyPEM, m.caPEM)
				if err != nil {
					t.Fatalf("NewServerTLSConfigFromPEM: %v", err)
				}
				return cfg
			}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := tt.cfg(t)
				if cfg.MaxVersion != tls.VersionTLS12 {
					t.Errorf("MaxVersion = %s, want TLS 1.2: IEEE 2030.5-2018 clauses 6.1 and 6.4 cap the server at TLS 1.2, with no opt-in", tls.VersionName(cfg.MaxVersion))
				}
				if cfg.ClientAuth != tls.RequireAnyClientCert || cfg.VerifyPeerCertificate == nil {
					t.Error("RequireAnyClientCert with a nil VerifyPeerCertificate accepts any client cert from any CA unverified: the two must always be paired")
				}
				if !cfg.SessionTicketsDisabled {
					t.Error("SessionTicketsDisabled must be true: a resumed session restores the peer cert from the ticket and skips VerifyPeerCertificate, bypassing the CSIP SAN check")
				}
			})
		}
	})

	t.Run("CCM constructors", func(t *testing.T) {
		tests := []struct {
			name string
			cfg  func(t *testing.T) *gotls.Config
		}{
			{"NewCCMServerConfig", func(t *testing.T) *gotls.Config {
				cfg, err := sepTLS.NewCCMServerConfig(m.certPath, m.keyPath, m.caPath)
				if err != nil {
					t.Fatalf("NewCCMServerConfig: %v", err)
				}
				return cfg
			}},
			{"NewCCMServerConfigWithExtraCAs", func(t *testing.T) *gotls.Config {
				cfg, err := sepTLS.NewCCMServerConfigWithExtraCAs(m.certPath, m.keyPath, m.caPath, nil)
				if err != nil {
					t.Fatalf("NewCCMServerConfigWithExtraCAs: %v", err)
				}
				return cfg
			}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := tt.cfg(t)
				if cfg.MaxVersion != gotls.VersionTLS12 {
					t.Errorf("MaxVersion = 0x%04x, want TLS 1.2 (0x%04x): IEEE 2030.5-2018 clauses 6.1 and 6.4 cap the server at TLS 1.2, with no opt-in", cfg.MaxVersion, gotls.VersionTLS12)
				}
				if cfg.ClientAuth != gotls.RequireAnyClientCert || cfg.VerifyPeerCertificate == nil {
					t.Error("RequireAnyClientCert with a nil VerifyPeerCertificate accepts any client cert from any CA unverified: the two must always be paired")
				}
				if !cfg.SessionTicketsDisabled {
					t.Error("SessionTicketsDisabled must be true: a resumed session restores the peer cert from the ticket and skips VerifyPeerCertificate, bypassing the CSIP SAN check")
				}
			})
		}
	})
}
