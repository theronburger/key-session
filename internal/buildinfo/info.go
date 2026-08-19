package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	Version   = "0.4.0"
	Commit    = "unknown"
	BuildDate = "unknown"
	Dirty     = "unknown"
)

type Info struct {
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	BuildDate    string `json:"build_date"`
	GoVersion    string `json:"go_version"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
	Dirty        bool   `json:"dirty"`
}

func Current() Info {
	current := Info{
		Version:      Version,
		Commit:       Commit,
		BuildDate:    BuildDate,
		GoVersion:    runtime.Version(),
		Platform:     runtime.GOOS,
		Architecture: runtime.GOARCH,
		Dirty:        Dirty == "true",
	}
	build, available := debug.ReadBuildInfo()
	if !available {
		return current
	}
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			if current.Commit == "unknown" && setting.Value != "" {
				current.Commit = setting.Value
			}
		case "vcs.time":
			if current.BuildDate == "unknown" && setting.Value != "" {
				current.BuildDate = setting.Value
			}
		case "vcs.modified":
			if Dirty == "unknown" {
				current.Dirty = setting.Value == "true"
			}
		}
	}
	return current
}

func ShortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return strings.TrimSpace(commit)
}
