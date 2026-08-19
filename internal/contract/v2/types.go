package v2

import "time"

const SchemaVersion = 2

type RuntimeDescriptor struct {
	SchemaVersion    int       `json:"schema_version"`
	Endpoint         string    `json:"endpoint"`
	Token            string    `json:"token"`
	DaemonInstanceID string    `json:"daemon_instance_id"`
	DaemonVersion    string    `json:"daemon_version"`
	PID              int       `json:"pid"`
	GeneratedAt      time.Time `json:"generated_at"`
}

type Handshake struct {
	SchemaVersion           int    `json:"schema_version"`
	DaemonInstanceID        string `json:"daemon_instance_id"`
	DaemonVersion           string `json:"daemon_version"`
	SupportedSchemaVersions []int  `json:"supported_schema_versions"`
}

type DaemonInfo struct {
	InstanceID string    `json:"instance_id"`
	Version    string    `json:"version"`
	StartedAt  time.Time `json:"started_at"`
}

type Profile struct {
	Name                string `json:"name"`
	EnvironmentVariable string `json:"environment_variable"`
	DefaultLeaseSeconds int64  `json:"default_lease_seconds"`
	IsDefault           bool   `json:"is_default"`
}

type Lease struct {
	ID                  string    `json:"id"`
	ConsumerID          string    `json:"consumer_id"`
	ConsumerLabel       string    `json:"consumer_label"`
	Profile             string    `json:"profile"`
	EnvironmentVariable string    `json:"environment_variable"`
	Reason              string    `json:"reason"`
	GrantedAt           time.Time `json:"granted_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

type Consumer struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Leases    []Lease   `json:"leases"`
}

type Event struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Profile       string    `json:"profile,omitempty"`
	ConsumerID    string    `json:"consumer_id,omitempty"`
	ConsumerLabel string    `json:"consumer_label,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	Detail        string    `json:"detail,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

type Snapshot struct {
	SchemaVersion int        `json:"schema_version"`
	Revision      int64      `json:"revision"`
	GeneratedAt   time.Time  `json:"generated_at"`
	Daemon        DaemonInfo `json:"daemon"`
	Profiles      []Profile  `json:"profiles"`
	Consumers     []Consumer `json:"consumers"`
	Events        []Event    `json:"events"`
}

// GrantRequest creates a consumer capability as part of the first grant when
// ConsumerToken is empty. Existing consumers present only ConsumerToken.
type GrantRequest struct {
	Profile                 string `json:"profile,omitempty"`
	ConsumerToken           string `json:"consumer_token,omitempty"`
	ConsumerLabel           string `json:"consumer_label,omitempty"`
	ConsumerDurationSeconds int64  `json:"consumer_duration_seconds,omitempty"`
	Reason                  string `json:"reason"`
	DurationSeconds         int64  `json:"duration_seconds,omitempty"`
}

type GrantResponse struct {
	ConsumerToken string   `json:"consumer_token,omitempty"`
	Consumer      Consumer `json:"consumer"`
	Lease         Lease    `json:"lease"`
}

type ConsumerRequest struct {
	ConsumerToken string `json:"consumer_token"`
}

type ConsumerStatusResponse struct {
	Consumer Consumer `json:"consumer"`
}

type RevokeRequest struct {
	ConsumerToken string `json:"consumer_token"`
	LeaseID       string `json:"lease_id,omitempty"`
}

type AdminRevokeRequest struct {
	ConsumerID string `json:"consumer_id"`
	LeaseID    string `json:"lease_id,omitempty"`
}

type ProfileRequest struct {
	Name                string `json:"name"`
	EnvironmentVariable string `json:"environment_variable"`
	DefaultLeaseSeconds int64  `json:"default_lease_seconds"`
	Secret              string `json:"secret"`
}

type ProfileUpdateRequest struct {
	EnvironmentVariable string `json:"environment_variable"`
	DefaultLeaseSeconds int64  `json:"default_lease_seconds"`
	Secret              string `json:"secret"`
	ManagementToken     string `json:"management_token"`
}

type ProfileManagementResponse struct {
	ManagementToken string    `json:"management_token"`
	Secret          string    `json:"secret"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type ProfileManagementRequest struct {
	ManagementToken string `json:"management_token"`
}

type DefaultProfileRequest struct {
	Name string `json:"name"`
}

type ExecRequest struct {
	ConsumerToken    string   `json:"consumer_token"`
	LeaseID          string   `json:"lease_id"`
	Arguments        []string `json:"arguments"`
	WorkingDirectory string   `json:"working_directory"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
}

type ExecResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

type DoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type DoctorReport struct {
	Healthy bool          `json:"healthy"`
	Checks  []DoctorCheck `json:"checks"`
}

type ErrorEnvelope struct {
	SchemaVersion int           `json:"schema_version"`
	Error         ContractError `json:"error"`
}

type ContractError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}
