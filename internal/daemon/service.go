package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/theronburger/key-session/internal/config"
	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
	"github.com/theronburger/key-session/internal/execution"
	"github.com/theronburger/key-session/internal/keychain"
)

const (
	minimumLease            = time.Minute
	maximumLease            = 24 * time.Hour
	defaultConsumerDuration = 24 * time.Hour
	minimumConsumerDuration = time.Hour
	maximumConsumerDuration = 7 * 24 * time.Hour
	managementSessionTTL    = 5 * time.Minute
)

var (
	profileNamePattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	environmentVariablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type activeLease struct {
	metadata contractv2.Lease
	secret   []byte
}

type consumerSession struct {
	metadata contractv2.Consumer
	leases   map[string]*activeLease
}

type profileManagementSession struct {
	profile   string
	secret    []byte
	expiresAt time.Time
}

type Service struct {
	mu         sync.Mutex
	config     config.Store
	audit      auditStore
	events     []contractv2.Event
	consumers  map[[sha256.Size]byte]*consumerSession
	management map[string]*profileManagementSession
	revision   int64
	instanceID string
	version    string
	startedAt  time.Time
}

func NewService(instanceID, version string, startedAt time.Time) (*Service, error) {
	configuration, err := config.DefaultStore()
	if err != nil {
		return nil, err
	}
	audit, err := defaultAuditStore()
	if err != nil {
		return nil, err
	}
	events, err := audit.load()
	if err != nil {
		return nil, err
	}
	return &Service{
		config: configuration, audit: audit, events: events, revision: 1,
		consumers: map[[sha256.Size]byte]*consumerSession{}, management: map[string]*profileManagementSession{},
		instanceID: instanceID, version: version, startedAt: startedAt,
	}, nil
}

func (service *Service) Close() {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.clearConsumersLocked()
	service.clearManagementSessionsLocked()
}

func (service *Service) Snapshot() (contractv2.Snapshot, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := time.Now()
	service.expireLocked(now)
	service.expireManagementSessionsLocked(now)
	configuration, err := service.config.Load()
	if err != nil {
		return contractv2.Snapshot{}, err
	}
	profiles := make([]contractv2.Profile, 0, len(configuration.Profiles))
	for _, name := range configuration.SortedProfileNames() {
		profile := configuration.Profiles[name]
		profiles = append(profiles, contractv2.Profile{
			Name: name, EnvironmentVariable: profile.EnvironmentVariable,
			DefaultLeaseSeconds: profile.DefaultLeaseSeconds,
			IsDefault:           name == configuration.DefaultProfile,
		})
	}
	consumers := make([]contractv2.Consumer, 0, len(service.consumers))
	for _, consumer := range service.consumers {
		consumers = append(consumers, publicConsumer(consumer))
	}
	sort.Slice(consumers, func(left, right int) bool { return consumers[left].CreatedAt.Before(consumers[right].CreatedAt) })
	events := append([]contractv2.Event(nil), service.events...)
	if events == nil {
		events = []contractv2.Event{}
	}
	sort.Slice(events, func(left, right int) bool { return events[left].OccurredAt.After(events[right].OccurredAt) })
	if len(events) > 50 {
		events = events[:50]
	}
	return contractv2.Snapshot{
		SchemaVersion: contractv2.SchemaVersion,
		Revision:      service.revision,
		GeneratedAt:   now,
		Daemon:        contractv2.DaemonInfo{InstanceID: service.instanceID, Version: service.version, StartedAt: service.startedAt},
		Profiles:      profiles, Consumers: consumers, Events: events,
	}, nil
}

func (service *Service) Grant(request contractv2.GrantRequest) (contractv2.GrantResponse, error) {
	request.ConsumerLabel = strings.TrimSpace(request.ConsumerLabel)
	request.Reason = strings.TrimSpace(request.Reason)
	if err := validatePromptField("reason", request.Reason, 240); err != nil {
		return contractv2.GrantResponse{}, err
	}

	now := time.Now()
	consumerID := ""
	consumerLabel := ""
	consumerExpiresAt := time.Time{}
	consumerToken := strings.TrimSpace(request.ConsumerToken)
	newConsumer := consumerToken == ""
	if newConsumer {
		if err := validatePromptField("consumer label", request.ConsumerLabel, 80); err != nil {
			return contractv2.GrantResponse{}, err
		}
		consumerDuration := time.Duration(request.ConsumerDurationSeconds) * time.Second
		if consumerDuration == 0 {
			consumerDuration = defaultConsumerDuration
		}
		if err := validateConsumerDuration(consumerDuration); err != nil {
			return contractv2.GrantResponse{}, err
		}
		var err error
		consumerToken, err = randomCapability("ksc_")
		if err != nil {
			return contractv2.GrantResponse{}, err
		}
		consumerID, err = randomIdentifier("consumer_")
		if err != nil {
			return contractv2.GrantResponse{}, err
		}
		consumerLabel = request.ConsumerLabel
		consumerExpiresAt = now.Add(consumerDuration)
	} else {
		if request.ConsumerLabel != "" || request.ConsumerDurationSeconds != 0 {
			return contractv2.GrantResponse{}, errors.New("consumer label and duration can only be set when creating a consumer")
		}
		service.mu.Lock()
		service.expireLocked(now)
		consumer, err := service.consumerForTokenLocked(consumerToken)
		if err == nil {
			consumerID = consumer.metadata.ID
			consumerLabel = consumer.metadata.Label
			consumerExpiresAt = consumer.metadata.ExpiresAt
		}
		service.mu.Unlock()
		if err != nil {
			return contractv2.GrantResponse{}, err
		}
	}

	configuration, err := service.config.Load()
	if err != nil {
		return contractv2.GrantResponse{}, err
	}
	name, profile, err := configuration.Resolve(request.Profile)
	if err != nil {
		return contractv2.GrantResponse{}, err
	}
	duration := time.Duration(request.DurationSeconds) * time.Second
	if duration == 0 {
		duration = time.Duration(profile.DefaultLeaseSeconds) * time.Second
	}
	if duration < minimumLease || duration > maximumLease {
		return contractv2.GrantResponse{}, errors.New("lease duration must be between 1m and 24h")
	}
	if remaining := time.Until(consumerExpiresAt); remaining < minimumLease {
		return contractv2.GrantResponse{}, errors.New("consumer session expires in less than one minute; create a new consumer")
	} else if duration > remaining {
		duration = remaining
	}
	message := grantApprovalMessage(consumerLabel, name, request.Reason, duration)
	secret, err := keychain.Read(name, message)
	if err != nil {
		return contractv2.GrantResponse{}, fmt.Errorf("read profile %q: %w", name, err)
	}
	leaseID, err := randomIdentifier("lease_")
	if err != nil {
		clearBytes(secret)
		return contractv2.GrantResponse{}, err
	}
	now = time.Now()
	lease := contractv2.Lease{
		ID: leaseID, ConsumerID: consumerID, ConsumerLabel: consumerLabel,
		Profile: name, EnvironmentVariable: profile.EnvironmentVariable, Reason: request.Reason,
		GrantedAt: now, ExpiresAt: minTime(now.Add(duration), consumerExpiresAt),
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	service.expireLocked(now)
	tokenHash := sha256.Sum256([]byte(consumerToken))
	consumer, found := service.consumers[tokenHash]
	if newConsumer {
		if found {
			clearBytes(secret)
			return contractv2.GrantResponse{}, errors.New("could not create a unique consumer capability")
		}
		consumer = &consumerSession{
			metadata: contractv2.Consumer{ID: consumerID, Label: consumerLabel, CreatedAt: now, ExpiresAt: consumerExpiresAt},
			leases:   map[string]*activeLease{},
		}
		service.consumers[tokenHash] = consumer
		service.recordLocked("consumer_started", "", consumerID, consumerLabel, "", "Consumer capability created")
	} else if !found || consumer.metadata.ID != consumerID {
		clearBytes(secret)
		return contractv2.GrantResponse{}, errors.New("consumer capability is invalid or expired")
	}
	for id, existing := range consumer.leases {
		if existing.metadata.Profile == name {
			service.clearLeaseLocked(consumer, id)
			service.recordLocked("revoked", existing.metadata.Profile, consumerID, consumerLabel, existing.metadata.Reason, "Replaced by a newer lease for this consumer and profile")
		}
	}
	consumer.leases[lease.ID] = &activeLease{metadata: lease, secret: secret}
	service.revision++
	service.recordLocked("granted", lease.Profile, consumerID, consumerLabel, lease.Reason, "Lease approved")
	response := contractv2.GrantResponse{Consumer: publicConsumer(consumer), Lease: lease}
	if newConsumer {
		response.ConsumerToken = consumerToken
	}
	return response, nil
}

func grantApprovalMessage(consumer, profile, reason string, duration time.Duration) string {
	return fmt.Sprintf(
		"Consumer: %s\n\nProfile: %s\nLease: %s\n\nReason: %s",
		consumer, profile, friendlyDuration(duration), reason,
	)
}

func (service *Service) ConsumerStatus(token string) (contractv2.ConsumerStatusResponse, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.expireLocked(time.Now())
	consumer, err := service.consumerForTokenLocked(strings.TrimSpace(token))
	if err != nil {
		return contractv2.ConsumerStatusResponse{}, err
	}
	return contractv2.ConsumerStatusResponse{Consumer: publicConsumer(consumer)}, nil
}

// Revoke removes one lease, or the entire consumer session when LeaseID is empty.
func (service *Service) Revoke(request contractv2.RevokeRequest) bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.expireLocked(time.Now())
	tokenHash := sha256.Sum256([]byte(strings.TrimSpace(request.ConsumerToken)))
	consumer, found := service.consumers[tokenHash]
	if !found {
		return false
	}
	if request.LeaseID == "" {
		service.clearConsumerLocked(tokenHash, consumer, "Consumer ended by its capability holder")
		return true
	}
	lease, found := consumer.leases[request.LeaseID]
	if !found {
		return false
	}
	metadata := lease.metadata
	service.clearLeaseLocked(consumer, request.LeaseID)
	service.recordLocked("revoked", metadata.Profile, consumer.metadata.ID, consumer.metadata.Label, metadata.Reason, "Lease revoked by its consumer")
	return true
}

// AdminRevoke is intentionally metadata-only administration for the native app;
// it can destroy access but never obtain or execute with a secret.
func (service *Service) AdminRevoke(request contractv2.AdminRevokeRequest) bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.expireLocked(time.Now())
	for tokenHash, consumer := range service.consumers {
		if consumer.metadata.ID != request.ConsumerID {
			continue
		}
		if request.LeaseID == "" {
			service.clearConsumerLocked(tokenHash, consumer, "Consumer ended from Key Session.app")
			return true
		}
		lease, found := consumer.leases[request.LeaseID]
		if !found {
			return false
		}
		metadata := lease.metadata
		service.clearLeaseLocked(consumer, request.LeaseID)
		service.recordLocked("revoked", metadata.Profile, consumer.metadata.ID, consumer.metadata.Label, metadata.Reason, "Lease revoked from Key Session.app")
		return true
	}
	return false
}

func (service *Service) Execute(ctx context.Context, request contractv2.ExecRequest) (contractv2.ExecResponse, error) {
	if len(request.Arguments) == 0 {
		return contractv2.ExecResponse{}, errors.New("exec requires a program")
	}
	if strings.TrimSpace(request.LeaseID) == "" {
		return contractv2.ExecResponse{}, errors.New("lease ID is required")
	}
	timeout := time.Duration(request.TimeoutSeconds) * time.Second
	if timeout < time.Second || timeout > 30*time.Minute {
		return contractv2.ExecResponse{}, errors.New("exec timeout must be between 1s and 30m")
	}
	service.mu.Lock()
	service.expireLocked(time.Now())
	consumer, err := service.consumerForTokenLocked(strings.TrimSpace(request.ConsumerToken))
	if err != nil {
		service.mu.Unlock()
		return contractv2.ExecResponse{}, err
	}
	lease, found := consumer.leases[request.LeaseID]
	if !found {
		service.mu.Unlock()
		return contractv2.ExecResponse{}, errors.New("lease is unavailable, expired, or belongs to another consumer")
	}
	secret := append([]byte(nil), lease.secret...)
	environmentVariable := lease.metadata.EnvironmentVariable
	service.mu.Unlock()
	defer clearBytes(secret)
	result := execution.ExecuteWithSecret(ctx, secret, environmentVariable, request.Arguments, request.WorkingDirectory, timeout)
	return contractv2.ExecResponse{ExitCode: result.ExitCode, Stdout: result.Stdout, Stderr: result.Stderr}, nil
}

func (service *Service) StoreProfile(request contractv2.ProfileRequest) error {
	if err := validateProfile(request.Name, request.EnvironmentVariable, request.DefaultLeaseSeconds); err != nil {
		return err
	}
	secret := []byte(request.Secret)
	defer clearBytes(secret)
	if len(secret) == 0 || bytes.IndexByte(secret, 0) >= 0 {
		return errors.New("secret must be non-empty and cannot contain a NUL byte")
	}
	if err := keychain.Store(request.Name, secret); err != nil {
		return fmt.Errorf("store profile %q: %w", request.Name, err)
	}
	configuration, err := service.config.Load()
	if err != nil {
		return err
	}
	configuration.Profiles[request.Name] = config.Profile{
		EnvironmentVariable: request.EnvironmentVariable,
		DefaultLeaseSeconds: request.DefaultLeaseSeconds,
	}
	if configuration.DefaultProfile == "" {
		configuration.DefaultProfile = request.Name
	}
	if err := service.config.Save(configuration); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.recordLocked("profile_saved", request.Name, "", "", "", "Profile stored in Keychain")
	return nil
}

func (service *Service) BeginProfileManagement(name string) (contractv2.ProfileManagementResponse, error) {
	configuration, err := service.config.Load()
	if err != nil {
		return contractv2.ProfileManagementResponse{}, err
	}
	if _, found := configuration.Profiles[name]; !found {
		return contractv2.ProfileManagementResponse{}, fmt.Errorf("profile %q is not configured", name)
	}
	secret, err := service.authorizeProfileManagement(name, "Open the human profile editor and view or change its password.")
	if err != nil {
		return contractv2.ProfileManagementResponse{}, err
	}
	token, err := randomCapability("ksm_")
	if err != nil {
		clearBytes(secret)
		return contractv2.ProfileManagementResponse{}, err
	}
	expiresAt := time.Now().Add(managementSessionTTL)
	service.mu.Lock()
	service.expireManagementSessionsLocked(time.Now())
	service.management[token] = &profileManagementSession{profile: name, secret: secret, expiresAt: expiresAt}
	service.recordLocked("profile_management_started", name, "", "Key Session.app", "Human profile management", "Five-minute management session approved")
	service.mu.Unlock()
	return contractv2.ProfileManagementResponse{ManagementToken: token, Secret: string(secret), ExpiresAt: expiresAt}, nil
}

func (service *Service) UpdateProfile(name string, request contractv2.ProfileUpdateRequest) error {
	if err := validateProfile(name, request.EnvironmentVariable, request.DefaultLeaseSeconds); err != nil {
		return err
	}
	secret := []byte(request.Secret)
	defer clearBytes(secret)
	if len(secret) == 0 || bytes.IndexByte(secret, 0) >= 0 {
		return errors.New("secret must be non-empty and cannot contain a NUL byte")
	}
	configuration, err := service.config.Load()
	if err != nil {
		return err
	}
	if _, found := configuration.Profiles[name]; !found {
		return fmt.Errorf("profile %q is not configured", name)
	}
	if err := service.consumeManagementSession(name, request.ManagementToken); err != nil {
		return err
	}
	if err := keychain.Store(name, secret); err != nil {
		return fmt.Errorf("update profile %q: %w", name, err)
	}
	configuration.Profiles[name] = config.Profile{EnvironmentVariable: request.EnvironmentVariable, DefaultLeaseSeconds: request.DefaultLeaseSeconds}
	if err := service.config.Save(configuration); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.clearProfileLeasesLocked(name, "Profile changed")
	service.recordLocked("profile_updated", name, "", "Key Session.app", "Human profile management", "Profile updated in an approved management session")
	return nil
}

func (service *Service) DeleteProfile(name, managementToken string) error {
	configuration, err := service.config.Load()
	if err != nil {
		return err
	}
	if _, found := configuration.Profiles[name]; !found {
		return fmt.Errorf("profile %q is not configured", name)
	}
	if err := service.consumeManagementSession(name, managementToken); err != nil {
		return err
	}
	service.mu.Lock()
	service.clearProfileLeasesLocked(name, "Profile removed")
	service.mu.Unlock()
	if err := keychain.Delete(name); err != nil {
		return fmt.Errorf("delete profile %q: %w", name, err)
	}
	delete(configuration.Profiles, name)
	if configuration.DefaultProfile == name {
		configuration.DefaultProfile = ""
		remaining := configuration.SortedProfileNames()
		if len(remaining) > 0 {
			configuration.DefaultProfile = remaining[0]
		}
	}
	if err := service.config.Save(configuration); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.recordLocked("profile_removed", name, "", "Key Session.app", "Human profile management", "Profile removed in an approved management session")
	return nil
}

func (service *Service) EndProfileManagement(name, managementToken string) bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	session, found := service.management[managementToken]
	if !found || session.profile != name {
		return false
	}
	clearBytes(session.secret)
	delete(service.management, managementToken)
	return true
}

func (service *Service) consumeManagementSession(name, token string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.expireManagementSessionsLocked(time.Now())
	session, found := service.management[token]
	if !found || session.profile != name {
		return errors.New("profile management authorization is missing or expired; reopen the profile editor")
	}
	clearBytes(session.secret)
	delete(service.management, token)
	return nil
}

func (service *Service) authorizeProfileManagement(name, reason string) ([]byte, error) {
	message := fmt.Sprintf("Reveal and manage the Key Session profile %s. %s", name, reason)
	secret, err := keychain.Read(name, message)
	if err != nil {
		return nil, fmt.Errorf("authorize management for profile %q: %w", name, err)
	}
	return secret, nil
}

func (service *Service) SetDefaultProfile(name string) error {
	configuration, err := service.config.Load()
	if err != nil {
		return err
	}
	if _, found := configuration.Profiles[name]; !found {
		return fmt.Errorf("profile %q is not configured", name)
	}
	configuration.DefaultProfile = name
	if err := service.config.Save(configuration); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.revision++
	return nil
}

func (service *Service) Doctor() contractv2.DoctorReport {
	checks := []contractv2.DoctorCheck{
		{Name: "Daemon", Status: "pass", Detail: "Authenticated loopback API is responding"},
		{Name: "Consumer isolation", Status: "info", Detail: "Exec and consumer revocation require an in-memory consumer capability and exact lease ID"},
	}
	runtimePaths, err := DefaultRuntimePaths()
	if err != nil {
		checks = append(checks, contractv2.DoctorCheck{Name: "Runtime directory", Status: "fail", Detail: err.Error()})
	} else {
		checks = append(checks,
			privatePathCheck("Runtime directory", runtimePaths.Directory, true, 0o700, false, ""),
			privatePathCheck("Endpoint descriptor", runtimePaths.Descriptor, false, 0o600, false, ""),
		)
	}
	checks = append(checks,
		privatePathCheck("Configuration", service.config.Path, false, 0o600, true, "No profiles configured yet"),
		privatePathCheck("Audit journal", service.audit.path, false, 0o600, true, "No audit events recorded yet"),
		contractv2.DoctorCheck{Name: "Approval policy", Status: "info", Detail: "Key Session requires biometrics-only LocalAuthentication before each Keychain read"},
	)
	healthy := true
	for _, check := range checks {
		if check.Status == "fail" {
			healthy = false
		}
	}
	return contractv2.DoctorReport{Healthy: healthy, Checks: checks}
}

func privatePathCheck(name, path string, directory bool, expectedMode os.FileMode, missingIsInfo bool, missingDetail string) contractv2.DoctorCheck {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) && missingIsInfo {
		return contractv2.DoctorCheck{Name: name, Status: "info", Detail: missingDetail}
	}
	if err != nil {
		return contractv2.DoctorCheck{Name: name, Status: "fail", Detail: err.Error()}
	}
	if (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		kind := "file"
		if directory {
			kind = "directory"
		}
		return contractv2.DoctorCheck{Name: name, Status: "fail", Detail: "Expected an owner-only regular " + kind}
	}
	if info.Mode().Perm() != expectedMode {
		return contractv2.DoctorCheck{Name: name, Status: "fail", Detail: fmt.Sprintf("Expected %04o, found %04o", expectedMode, info.Mode().Perm())}
	}
	return contractv2.DoctorCheck{Name: name, Status: "pass", Detail: fmt.Sprintf("Protected with mode %04o", expectedMode)}
}

func (service *Service) consumerForTokenLocked(token string) (*consumerSession, error) {
	if token == "" {
		return nil, errors.New("consumer capability is required")
	}
	consumer, found := service.consumers[sha256.Sum256([]byte(token))]
	if !found {
		return nil, errors.New("consumer capability is invalid or expired")
	}
	return consumer, nil
}

func publicConsumer(consumer *consumerSession) contractv2.Consumer {
	leases := make([]contractv2.Lease, 0, len(consumer.leases))
	for _, lease := range consumer.leases {
		leases = append(leases, lease.metadata)
	}
	sort.Slice(leases, func(left, right int) bool { return leases[left].GrantedAt.Before(leases[right].GrantedAt) })
	return contractv2.Consumer{
		ID: consumer.metadata.ID, Label: consumer.metadata.Label,
		CreatedAt: consumer.metadata.CreatedAt, ExpiresAt: consumer.metadata.ExpiresAt, Leases: leases,
	}
}

func (service *Service) expireLocked(now time.Time) {
	for tokenHash, consumer := range service.consumers {
		for leaseID, lease := range consumer.leases {
			if now.Before(lease.metadata.ExpiresAt) {
				continue
			}
			metadata := lease.metadata
			service.clearLeaseLocked(consumer, leaseID)
			service.recordLocked("expired", metadata.Profile, consumer.metadata.ID, consumer.metadata.Label, metadata.Reason, "Lease expired automatically")
		}
		if now.Before(consumer.metadata.ExpiresAt) {
			continue
		}
		service.clearConsumerLocked(tokenHash, consumer, "Consumer capability expired automatically")
	}
}

func (service *Service) clearLeaseLocked(consumer *consumerSession, leaseID string) {
	lease, found := consumer.leases[leaseID]
	if !found {
		return
	}
	clearBytes(lease.secret)
	delete(consumer.leases, leaseID)
	service.revision++
}

func (service *Service) clearConsumerLocked(tokenHash [sha256.Size]byte, consumer *consumerSession, detail string) {
	for leaseID, lease := range consumer.leases {
		metadata := lease.metadata
		service.clearLeaseLocked(consumer, leaseID)
		service.recordLocked("revoked", metadata.Profile, consumer.metadata.ID, consumer.metadata.Label, metadata.Reason, detail)
	}
	delete(service.consumers, tokenHash)
	service.recordLocked("consumer_ended", "", consumer.metadata.ID, consumer.metadata.Label, "", detail)
}

func (service *Service) clearConsumersLocked() {
	for tokenHash, consumer := range service.consumers {
		for leaseID := range consumer.leases {
			service.clearLeaseLocked(consumer, leaseID)
		}
		delete(service.consumers, tokenHash)
	}
}

func (service *Service) clearProfileLeasesLocked(profile, detail string) {
	for _, consumer := range service.consumers {
		for leaseID, lease := range consumer.leases {
			if lease.metadata.Profile != profile {
				continue
			}
			metadata := lease.metadata
			service.clearLeaseLocked(consumer, leaseID)
			service.recordLocked("revoked", metadata.Profile, consumer.metadata.ID, consumer.metadata.Label, metadata.Reason, detail)
		}
	}
}

func (service *Service) expireManagementSessionsLocked(now time.Time) {
	for token, session := range service.management {
		if now.Before(session.expiresAt) {
			continue
		}
		clearBytes(session.secret)
		delete(service.management, token)
	}
}

func (service *Service) clearManagementSessionsLocked() {
	for token, session := range service.management {
		clearBytes(session.secret)
		delete(service.management, token)
	}
}

func (service *Service) recordLocked(kind, profile, consumerID, consumerLabel, reason, detail string) {
	service.revision++
	event := contractv2.Event{
		ID: fmt.Sprintf("evt_%d_%d", time.Now().UnixNano(), service.revision), Kind: kind,
		Profile: profile, ConsumerID: consumerID, ConsumerLabel: consumerLabel,
		Reason: reason, Detail: detail, OccurredAt: time.Now(),
	}
	service.events = append(service.events, event)
	if len(service.events) > maximumAuditEvents {
		service.events = service.events[len(service.events)-maximumAuditEvents:]
	}
	_ = service.audit.save(service.events)
}

func validatePromptField(name, value string, maximumCharacters int) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len([]rune(value)) > maximumCharacters {
		return fmt.Errorf("%s must be at most %d characters", name, maximumCharacters)
	}
	for _, character := range value {
		if character < ' ' || character == 0x7f {
			return fmt.Errorf("%s cannot contain control characters", name)
		}
	}
	return nil
}

func validateConsumerDuration(duration time.Duration) error {
	if duration < minimumConsumerDuration || duration > maximumConsumerDuration {
		return errors.New("consumer duration must be between 1h and 7d")
	}
	return nil
}

func validateEnvironment(name string) error {
	if !environmentVariablePattern.MatchString(name) {
		return errors.New("environment variable must look like API_TOKEN or MONGODB_URI")
	}
	upper := strings.ToUpper(name)
	dangerous := map[string]bool{"PATH": true, "IFS": true, "BASH_ENV": true, "ENV": true, "PROMPT_COMMAND": true, "PYTHONPATH": true, "PYTHONHOME": true, "NODE_OPTIONS": true, "RUBYOPT": true}
	if dangerous[upper] || strings.HasPrefix(upper, "LD_") || strings.HasPrefix(upper, "DYLD_") {
		return fmt.Errorf("refusing dangerous environment variable %q", name)
	}
	return nil
}

func validateProfile(name, environmentVariable string, defaultLeaseSeconds int64) error {
	if !profileNamePattern.MatchString(name) {
		return errors.New("profile names must be 1-64 characters using letters, numbers, dots, underscores, or hyphens")
	}
	if err := validateEnvironment(environmentVariable); err != nil {
		return err
	}
	duration := time.Duration(defaultLeaseSeconds) * time.Second
	if duration < minimumLease || duration > maximumLease {
		return errors.New("lease duration must be between 1m and 24h")
	}
	return nil
}

func friendlyDuration(duration time.Duration) string {
	duration = duration.Round(time.Second)
	if duration >= time.Hour {
		return fmt.Sprintf("%dh %dm", int(duration/time.Hour), int(duration%time.Hour/time.Minute))
	}
	return fmt.Sprintf("%dm", int(duration/time.Minute))
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func clearBytes(contents []byte) {
	for index := range contents {
		contents[index] = 0
	}
}
