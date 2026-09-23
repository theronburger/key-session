package daemon

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/theronburger/key-session/internal/config"
	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func sshFixture(t *testing.T) (*Service, ssh.PublicKey, []byte) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte(base64.StdEncoding.EncodeToString(private.Seed()))
	clearBytes(private)
	now := time.Now()
	service := &Service{
		config: config.Store{Path: filepath.Join(t.TempDir(), "config.json")},
		consumers: map[[sha256.Size]byte]*consumerSession{
			sha256.Sum256([]byte("test-consumer")): {
				metadata: contractv2.Consumer{ID: "consumer", Label: "Test", ExpiresAt: now.Add(time.Hour)},
				leases: map[string]*activeLease{
					"lease": {secret: secret, metadata: contractv2.Lease{ID: "lease", Profile: "infrastructure", Kind: "ssh", ExpiresAt: now.Add(time.Hour)}},
				},
			},
		},
		management: map[string]*profileManagementSession{},
	}
	if err := service.config.Save(config.Config{Profiles: map[string]config.Profile{
		"infrastructure": {Kind: "ssh", PublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), DefaultLeaseSeconds: 3600},
	}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	return service, key, secret
}

func authenticationPayload(key ssh.PublicKey) []byte {
	return ssh.Marshal(struct {
		Session   []byte
		Message   byte
		User      string
		Service   string
		Method    string
		Signature bool
		Algorithm string
		Key       []byte
	}{bytes.Repeat([]byte{1}, 32), 50, "test", "ssh-connection", "publickey", true, key.Type(), key.Marshal()})
}

func TestSSHLeaseSignsAndRevocationClearsAccess(t *testing.T) {
	service, public, secret := sshFixture(t)
	signer := leaseSSHAgent{service}
	keys, err := signer.List()
	if err != nil || len(keys) != 1 {
		t.Fatalf("list: %v %v", keys, err)
	}
	payload := authenticationPayload(public)
	signature, err := signer.Sign(public, payload)
	if err != nil || public.Verify(payload, signature) != nil {
		t.Fatalf("signature failed: %v", err)
	}
	if !service.Revoke(contractv2.RevokeRequest{ConsumerToken: "test-consumer", LeaseID: "lease"}) {
		t.Fatal("revoke failed")
	}
	if _, err := signer.Sign(public, payload); err == nil {
		t.Fatal("revoked lease signed")
	}
	keys, err = signer.List()
	if err != nil || len(keys) != 0 || !bytes.Equal(secret, make([]byte, len(secret))) {
		t.Fatal("revocation did not clear the identity")
	}
}

func TestSSHExpiryAndShutdownBlockSigning(t *testing.T) {
	for _, mode := range []string{"lease", "consumer", "shutdown", "admin"} {
		t.Run(mode, func(t *testing.T) {
			service, public, secret := sshFixture(t)
			consumer := service.consumers[sha256.Sum256([]byte("test-consumer"))]
			switch mode {
			case "lease":
				consumer.leases["lease"].metadata.ExpiresAt = time.Now().Add(-time.Second)
			case "consumer":
				consumer.metadata.ExpiresAt = time.Now().Add(-time.Second)
			case "shutdown":
				service.Close()
			case "admin":
				service.AdminRevoke(contractv2.AdminRevokeRequest{ConsumerID: "consumer"})
			}
			if _, err := (leaseSSHAgent{service}).Sign(public, authenticationPayload(public)); err == nil {
				t.Fatal("inactive identity signed")
			}
			if !bytes.Equal(secret, make([]byte, len(secret))) {
				t.Fatal("secret not cleared")
			}
		})
	}
}

func TestSSHOnlySignsAuthenticationForAnActiveIdentity(t *testing.T) {
	service, public, secret := sshFixture(t)
	signer := leaseSSHAgent{service}
	if _, err := signer.Sign(public, []byte("arbitrary document")); err == nil {
		t.Fatal("signed arbitrary data")
	}
	if _, err := signer.Sign(public, append(authenticationPayload(public), 0)); err == nil {
		t.Fatal("accepted trailing data")
	}
	_, other, _ := sshFixture(t)
	if _, err := signer.Sign(other, authenticationPayload(other)); err == nil {
		t.Fatal("signed with unknown identity")
	}
	if err := validateSSHSecret(secret, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(other)))); err == nil {
		t.Fatal("accepted mismatched configured key")
	}
	if _, err := sshPrivateKey([]byte("invalid")); err == nil {
		t.Fatal("accepted invalid seed")
	}
	consumer := service.consumers[sha256.Sum256([]byte("test-consumer"))]
	consumer.leases["lease"].metadata.Kind = ""
	keys, err := signer.List()
	if err != nil || len(keys) != 0 {
		t.Fatal("exposed an ordinary secret profile")
	}
	if _, err := signer.Sign(public, authenticationPayload(public)); err == nil {
		t.Fatal("signed with ordinary secret profile")
	}
}

func TestSSHLeaseCannotBeExportedOrOverwritten(t *testing.T) {
	service, _, _ := sshFixture(t)
	_, err := service.Execute(context.Background(), contractv2.ExecRequest{
		ConsumerToken: "test-consumer", LeaseID: "lease", Arguments: []string{"/usr/bin/env"}, TimeoutSeconds: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "only sign") {
		t.Fatalf("export attempt: %v", err)
	}
	if _, err := service.BeginProfileManagement("infrastructure"); err == nil {
		t.Fatal("private key reveal allowed")
	}
	if err := service.StoreProfile(contractv2.ProfileRequest{Name: "infrastructure", EnvironmentVariable: "SECRET", Secret: "replacement", DefaultLeaseSeconds: 3600}); err == nil {
		t.Fatal("secret profile overwrote SSH identity")
	}
	if err := service.UpdateProfile("infrastructure", contractv2.ProfileUpdateRequest{EnvironmentVariable: "SECRET", Secret: "replacement", DefaultLeaseSeconds: 3600}); err == nil {
		t.Fatal("editor overwrote SSH identity")
	}
	if _, err := service.CreateSSHProfile(contractv2.SSHProfileRequest{Name: "infrastructure", DefaultLeaseSeconds: 3600}); err == nil {
		t.Fatal("SSH setup overwrote an identity")
	}
	if _, err := service.CreateSSHProfile(contractv2.SSHProfileRequest{Name: "bad/name", DefaultLeaseSeconds: 3600}); err == nil {
		t.Fatal("invalid profile name accepted")
	}
	if _, err := service.CreateSSHProfile(contractv2.SSHProfileRequest{Name: "valid", DefaultLeaseSeconds: 0}); err == nil {
		t.Fatal("invalid lease duration accepted")
	}
}

func TestSSHCreationReturnsOnlyPublicMetadata(t *testing.T) {
	service, _, _ := sshFixture(t)
	var stored []byte
	profile, err := service.createSSHProfile(contractv2.SSHProfileRequest{Name: "new-identity", DefaultLeaseSeconds: 3600}, func(name string, secret []byte) error {
		if name != "new-identity" {
			t.Fatal("wrong storage account")
		}
		stored = append([]byte(nil), secret...)
		return nil
	})
	if err != nil || profile.Kind != "ssh" || profile.EnvironmentVariable != "" {
		t.Fatalf("creation failed: %v", err)
	}
	defer clearBytes(stored)
	if err := validateSSHSecret(stored, profile.PublicKey); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil || bytes.Contains(encoded, stored) {
		t.Fatal("snapshot exposed private material")
	}
	for _, consumer := range snapshot.Consumers {
		for _, lease := range consumer.Leases {
			if lease.Profile == "new-identity" {
				t.Fatal("creation implicitly granted signing")
			}
		}
	}
}

func TestSSHOtherActiveLeaseKeepsIdentityAvailable(t *testing.T) {
	service, key, secret := sshFixture(t)
	service.consumers[sha256.Sum256([]byte("other"))] = &consumerSession{
		metadata: contractv2.Consumer{ID: "other", ExpiresAt: time.Now().Add(time.Hour)},
		leases: map[string]*activeLease{"second": {
			secret:   append([]byte(nil), secret...),
			metadata: contractv2.Lease{ID: "second", Profile: "infrastructure", Kind: "ssh", ExpiresAt: time.Now().Add(time.Hour)},
		}},
	}
	signer := leaseSSHAgent{service}
	keys, _ := signer.List()
	if len(keys) != 1 {
		t.Fatal("duplicate identity listed")
	}
	service.Revoke(contractv2.RevokeRequest{ConsumerToken: "test-consumer"})
	if _, err := signer.Sign(key, authenticationPayload(key)); err != nil {
		t.Fatalf("other lease should still work: %v", err)
	}
	service.Revoke(contractv2.RevokeRequest{ConsumerToken: "other"})
	if _, err := signer.Sign(key, authenticationPayload(key)); err == nil {
		t.Fatal("signed after last lease ended")
	}
}

func testSSHSocket(t *testing.T, service *Service) string {
	t.Helper()
	// macOS Unix socket paths are limited to 104 bytes.
	directory, err := os.MkdirTemp("/tmp", "ks-ssh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "agent.sock")
	listener, err := service.listenSSHAgent(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("socket permissions")
	}
	return path
}

func TestSSHProtocolRevokesAnAlreadyConnectedClient(t *testing.T) {
	service, key, _ := sshFixture(t)
	connection, err := net.Dial("unix", testSSHSocket(t, service))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	client := agent.NewClient(connection)
	if _, err := client.Sign(key, authenticationPayload(key)); err != nil {
		t.Fatal(err)
	}
	if err := client.RemoveAll(); err == nil {
		t.Fatal("agent protocol changed lease state")
	}
	if err := client.Lock([]byte("password")); err == nil {
		t.Fatal("agent lock bypassed lease management")
	}
	if _, err := client.Extension("session-bind@openssh.com", nil); err == nil {
		t.Fatal("claimed unsupported destination binding")
	}
	service.Revoke(contractv2.RevokeRequest{ConsumerToken: "test-consumer"})
	if _, err := client.Sign(key, authenticationPayload(key)); err == nil {
		t.Fatal("connected client signed after revoke")
	}
}

func TestSSHSocketDoesNotReplaceRegularFile(t *testing.T) {
	service, _, _ := sshFixture(t)
	path := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(path, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.listenSSHAgent(path); err == nil {
		t.Fatal("replaced existing file")
	}
	contents, _ := os.ReadFile(path)
	if string(contents) != "preserve" {
		t.Fatal("existing file changed")
	}
}

func TestOrdinaryOpenSSHUsesLeasedIdentity(t *testing.T) {
	service, public, _ := sshFixture(t)
	socket := testSSHSocket(t, service)
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostKey, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if !bytes.Equal(key.Marshal(), public.Marshal()) {
			return nil, fmt.Errorf("unknown key")
		}
		return &ssh.Permissions{}, nil
	}}
	serverConfig.AddHostKey(hostKey)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	go serveSSHTest(listener, serverConfig)
	directory := t.TempDir()
	publicPath := filepath.Join(directory, "identity.pub")
	if err := os.WriteFile(publicPath, ssh.MarshalAuthorizedKey(public), 0o600); err != nil {
		t.Fatal(err)
	}
	knownHosts := filepath.Join(directory, "known_hosts")
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	entry := "[127.0.0.1]:" + port + " " + string(ssh.MarshalAuthorizedKey(hostKey.PublicKey()))
	if err := os.WriteFile(knownHosts, []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}
	connect := func() ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "/usr/bin/ssh", "-F", "/dev/null", "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes",
			"-o", "IdentityAgent="+socket, "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile="+knownHosts,
			"-o", "PreferredAuthentications=publickey", "-i", publicPath, "-p", port, "test@127.0.0.1", "true").CombinedOutput()
	}
	output, err := connect()
	if err != nil || !strings.Contains(string(output), "SSH_LEASE_OK\n") {
		t.Fatalf("OpenSSH failed: %v %s", err, output)
	}
	service.Revoke(contractv2.RevokeRequest{ConsumerToken: "test-consumer"})
	output, err = connect()
	if err == nil || !strings.Contains(string(output), "Permission denied") {
		t.Fatalf("expected authentication failure: %v %s", err, output)
	}
}

func serveSSHTest(listener net.Listener, configuration *ssh.ServerConfig) {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = connection.Close() }()
			_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
			server, channels, requests, err := ssh.NewServerConn(connection, configuration)
			if err != nil {
				return
			}
			defer func() { _ = server.Close() }()
			go ssh.DiscardRequests(requests)
			for request := range channels {
				channel, requests, err := request.Accept()
				if err != nil {
					return
				}
				for request := range requests {
					_ = request.Reply(request.Type == "exec", nil)
					if request.Type == "exec" {
						_, _ = channel.Write([]byte("SSH_LEASE_OK\n"))
						_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{0}))
						_ = channel.Close()
						return
					}
				}
			}
		}()
	}
}
