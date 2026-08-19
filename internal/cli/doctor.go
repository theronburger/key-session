package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/theronburger/key-session/internal/apiclient"
	"github.com/theronburger/key-session/internal/buildinfo"
	"github.com/theronburger/key-session/internal/config"
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func newDoctorCommand() *cobra.Command {
	var outputJSON bool
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Check the local installation without reading secrets",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDoctor(outputJSON)
		},
	}
	command.Flags().BoolVar(&outputJSON, "json", false, "emit machine-readable JSON")
	return command
}

func runDoctor(outputJSON bool) error {
	checks := []doctorCheck{{Name: "platform", Status: "pass", Detail: runtime.GOOS + "/" + runtime.GOARCH}}
	if runtime.GOOS != "darwin" {
		checks[0] = doctorCheck{Name: "platform", Status: "fail", Detail: "key-session requires macOS"}
	}

	executable, executableError := os.Executable()
	if executableError != nil {
		checks = append(checks, doctorCheck{Name: "application bundle", Status: "fail", Detail: executableError.Error()})
	} else {
		if resolvedExecutable, err := filepath.EvalSymlinks(executable); err == nil {
			executable = resolvedExecutable
		}
		checks = append(checks, inspectApplicationBundle(executable))
	}

	store, err := config.DefaultStore()
	if err != nil {
		checks = append(checks, doctorCheck{Name: "configuration", Status: "fail", Detail: err.Error()})
	} else if info, statError := os.Stat(store.Path); os.IsNotExist(statError) {
		checks = append(checks, doctorCheck{Name: "configuration", Status: "info", Detail: "no profiles configured yet"})
	} else if statError != nil {
		checks = append(checks, doctorCheck{Name: "configuration", Status: "fail", Detail: statError.Error()})
	} else if info.Mode().Perm() != 0o600 {
		checks = append(checks, doctorCheck{Name: "configuration permissions", Status: "fail", Detail: fmt.Sprintf("expected 0600, found %04o", info.Mode().Perm())})
	} else {
		checks = append(checks, doctorCheck{Name: "configuration permissions", Status: "pass", Detail: "0600"})
	}

	checks = append(checks, doctorCheck{Name: "approval policy", Status: "info", Detail: "grants require biometrics-only LocalAuthentication; availability is checked when approval begins"})
	client, daemonError := apiclient.Connect(context.Background())
	if daemonError != nil {
		checks = append(checks, doctorCheck{Name: "access daemon", Status: "fail", Detail: daemonError.Error()})
	} else if snapshot, snapshotError := client.Snapshot(context.Background()); snapshotError != nil {
		checks = append(checks, doctorCheck{Name: "access daemon", Status: "fail", Detail: snapshotError.Error()})
	} else {
		leaseCount := 0
		for _, consumer := range snapshot.Consumers {
			leaseCount += len(consumer.Leases)
		}
		checks = append(checks, doctorCheck{Name: "access daemon", Status: "pass", Detail: fmt.Sprintf("connected; %d consumer(s), %d active lease(s)", len(snapshot.Consumers), leaseCount)})
	}

	healthy := true
	for _, check := range checks {
		if check.Status == "fail" {
			healthy = false
		}
	}
	if outputJSON {
		return printJSON(map[string]any{"healthy": healthy, "version": buildinfo.Current(), "checks": checks})
	}
	for _, check := range checks {
		fmt.Printf("%-5s %-26s %s\n", strings.ToUpper(check.Status), check.Name, check.Detail)
	}
	if !healthy {
		return fmt.Errorf("doctor found one or more installation problems")
	}
	fmt.Println("Doctor: healthy")
	return nil
}

func inspectApplicationBundle(executable string) doctorCheck {
	applicationBundle := findApplicationBundle(executable)
	if applicationBundle == "" {
		return doctorCheck{Name: "application bundle", Status: "warn", Detail: "not running from Key Session.app; the system Keychain prompt may use a generic executable icon"}
	}
	if err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", applicationBundle).Run(); err != nil {
		return doctorCheck{Name: "code signature", Status: "fail", Detail: "bundle signature verification failed"}
	}
	signatureDetails, _ := exec.Command("/usr/bin/codesign", "-dv", "--verbose=4", applicationBundle).CombinedOutput()
	if strings.Contains(string(signatureDetails), "Signature=adhoc") {
		return doctorCheck{Name: "code signature", Status: "warn", Detail: "valid ad hoc signature; published builds use Developer ID signing and notarization"}
	}
	return doctorCheck{Name: "code signature", Status: "pass", Detail: applicationBundle}
}

func findApplicationBundle(executable string) string {
	marker := ".app/Contents/MacOS/"
	markerIndex := strings.Index(executable, marker)
	if markerIndex < 0 {
		return ""
	}
	return executable[:markerIndex+len(".app")]
}
