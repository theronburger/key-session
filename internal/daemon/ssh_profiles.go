package daemon

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/theronburger/key-session/internal/config"
	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
	"github.com/theronburger/key-session/internal/keychain"
	"golang.org/x/crypto/ssh"
)

func (service *Service) CreateSSHProfile(request contractv2.SSHProfileRequest) (contractv2.Profile, error) {
	return service.createSSHProfile(request, keychain.Store)
}

func (service *Service) createSSHProfile(request contractv2.SSHProfileRequest, store func(string, []byte) error) (contractv2.Profile, error) {
	if err := validateProfile(request.Name, "SSH_SIGNING", request.DefaultLeaseSeconds); err != nil {
		return contractv2.Profile{}, err
	}
	service.profileMu.Lock()
	defer service.profileMu.Unlock()
	configuration, err := service.config.Load()
	if err != nil {
		return contractv2.Profile{}, err
	}
	if _, exists := configuration.Profiles[request.Name]; exists {
		return contractv2.Profile{}, errors.New("profile already exists; choose a new name to create an SSH identity")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return contractv2.Profile{}, err
	}
	defer clearBytes(private)
	seed := private.Seed()
	defer clearBytes(seed)
	secret := make([]byte, base64.StdEncoding.EncodedLen(len(seed)))
	base64.StdEncoding.Encode(secret, seed)
	defer clearBytes(secret)
	publicKey, err := ssh.NewPublicKey(public)
	if err != nil {
		return contractv2.Profile{}, err
	}
	profile := config.Profile{
		Kind: "ssh", PublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))),
		DefaultLeaseSeconds: request.DefaultLeaseSeconds,
	}
	if err := store(request.Name, secret); err != nil {
		return contractv2.Profile{}, err
	}
	configuration.Profiles[request.Name] = profile
	if err := service.config.Save(configuration); err != nil {
		return contractv2.Profile{}, err
	}
	service.mu.Lock()
	service.recordLocked("profile_saved", request.Name, "", "", "", "SSH signing identity generated in Keychain; private key is not exportable through the API")
	service.mu.Unlock()
	return contractv2.Profile{Name: request.Name, Kind: profile.Kind, PublicKey: profile.PublicKey, DefaultLeaseSeconds: profile.DefaultLeaseSeconds}, nil
}

func sshPrivateKey(secret []byte) (ed25519.PrivateKey, error) {
	seed := make([]byte, base64.StdEncoding.DecodedLen(len(secret)))
	defer clearBytes(seed)
	count, err := base64.StdEncoding.Decode(seed, secret)
	if err != nil || count != ed25519.SeedSize {
		return nil, errors.New("invalid SSH signing identity")
	}
	return ed25519.NewKeyFromSeed(seed[:count]), nil
}

func validateSSHSecret(secret []byte, expected string) error {
	private, err := sshPrivateKey(secret)
	if err != nil {
		return err
	}
	defer clearBytes(private)
	public, err := ssh.NewPublicKey(private.Public())
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(ssh.MarshalAuthorizedKey(public))) != expected {
		return errors.New("SSH identity does not match its configured public key")
	}
	return nil
}
