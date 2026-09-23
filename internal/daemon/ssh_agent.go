package daemon

import (
	"bytes"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var errSSHAgentReadOnly = errors.New("manage SSH access through Key Session leases")

type leaseSSHAgent struct{ service *Service }

func (signer leaseSSHAgent) List() ([]*agent.Key, error) {
	signer.service.mu.Lock()
	defer signer.service.mu.Unlock()
	signer.service.expireLocked(time.Now())
	keys := []*agent.Key{}
	seen := map[string]bool{}
	for _, consumer := range signer.service.consumers {
		for _, lease := range consumer.leases {
			if lease.metadata.Kind != "ssh" || seen[lease.metadata.Profile] {
				continue
			}
			private, err := sshPrivateKey(lease.secret)
			if err != nil {
				return nil, err
			}
			public, err := ssh.NewPublicKey(private.Public())
			clearBytes(private)
			if err != nil {
				return nil, err
			}
			keys = append(keys, &agent.Key{Format: public.Type(), Blob: public.Marshal(), Comment: "key-session:" + lease.metadata.Profile})
			seen[lease.metadata.Profile] = true
		}
	}
	return keys, nil
}

func (signer leaseSSHAgent) Sign(public ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	if !sshAuthenticationRequest(public, data) {
		return nil, errors.New("only SSH public-key authentication can be signed")
	}
	signer.service.mu.Lock()
	defer signer.service.mu.Unlock()
	signer.service.expireLocked(time.Now())
	for _, consumer := range signer.service.consumers {
		for _, lease := range consumer.leases {
			if lease.metadata.Kind != "ssh" {
				continue
			}
			private, err := sshPrivateKey(lease.secret)
			if err != nil {
				return nil, err
			}
			key, err := ssh.NewSignerFromKey(private)
			if err != nil {
				clearBytes(private)
				return nil, err
			}
			if !bytes.Equal(public.Marshal(), key.PublicKey().Marshal()) {
				clearBytes(private)
				continue
			}
			signature, err := key.Sign(rand.Reader, data)
			clearBytes(private)
			return signature, err
		}
	}
	return nil, errors.New("SSH signing requires an active approved lease")
}

func sshAuthenticationRequest(public ssh.PublicKey, data []byte) bool {
	var request struct {
		Session   []byte
		Message   byte
		User      string
		Service   string
		Method    string
		Signature bool
		Algorithm string
		Key       []byte
		Rest      []byte `ssh:"rest"`
	}
	if len(data) > 16384 || ssh.Unmarshal(data, &request) != nil ||
		len(request.Session) < 16 || request.Message != 50 || request.User == "" ||
		request.Service != "ssh-connection" || !request.Signature ||
		request.Algorithm != ssh.KeyAlgoED25519 || public.Type() != ssh.KeyAlgoED25519 ||
		!bytes.Equal(request.Key, public.Marshal()) {
		return false
	}
	switch request.Method {
	case "publickey":
		return len(request.Rest) == 0
	case "publickey-hostbound-v00@openssh.com":
		var bound struct{ HostKey []byte }
		if ssh.Unmarshal(request.Rest, &bound) != nil {
			return false
		}
		_, err := ssh.ParsePublicKey(bound.HostKey)
		return err == nil
	default:
		return false
	}
}

func (leaseSSHAgent) Add(agent.AddedKey) error       { return errSSHAgentReadOnly }
func (leaseSSHAgent) Remove(ssh.PublicKey) error     { return errSSHAgentReadOnly }
func (leaseSSHAgent) RemoveAll() error               { return errSSHAgentReadOnly }
func (leaseSSHAgent) Lock([]byte) error              { return errSSHAgentReadOnly }
func (leaseSSHAgent) Unlock([]byte) error            { return errSSHAgentReadOnly }
func (leaseSSHAgent) Signers() ([]ssh.Signer, error) { return nil, errSSHAgentReadOnly }

type sshAgentListener struct {
	net.Listener
	mu          sync.Mutex
	connections map[net.Conn]bool
	closed      bool
}

func (listener *sshAgentListener) Close() error {
	err := listener.Listener.Close()
	listener.mu.Lock()
	defer listener.mu.Unlock()
	listener.closed = true
	for connection := range listener.connections {
		_ = connection.Close()
	}
	return err
}

func (service *Service) listenSSHAgent(path string) (*sshAgentListener, error) {
	directory := filepath.Dir(path)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("SSH agent requires a private runtime directory")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("refusing to replace a non-socket SSH agent path")
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	socket, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = socket.Close()
		return nil, err
	}
	listener := &sshAgentListener{Listener: socket, connections: map[net.Conn]bool{}}
	go func() {
		for {
			connection, err := socket.Accept()
			if err != nil {
				return
			}
			listener.mu.Lock()
			if listener.closed || len(listener.connections) >= 32 {
				_ = connection.Close()
				listener.mu.Unlock()
				continue
			}
			listener.connections[connection] = true
			listener.mu.Unlock()
			go func() {
				defer func() {
					_ = connection.Close()
					listener.mu.Lock()
					delete(listener.connections, connection)
					listener.mu.Unlock()
				}()
				_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
				_ = agent.ServeAgent(leaseSSHAgent{service}, connection)
			}()
		}
	}()
	return listener, nil
}
