package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAPIURL      = "https://api.github.com/repos/theronburger/key-session/releases/latest"
	DefaultReleasePage = "https://github.com/theronburger/key-session/releases/latest"
	cacheLifetime      = 24 * time.Hour
)

type Result struct {
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version"`
	UpdateAvailable bool      `json:"update_available"`
	ReleaseURL      string    `json:"release_url"`
	CheckedAt       time.Time `json:"checked_at"`
}

type Checker struct {
	APIURL    string
	CachePath string
	Client    *http.Client
	Now       func() time.Time
}

type cachedRelease struct {
	LatestVersion string    `json:"latest_version"`
	ReleaseURL    string    `json:"release_url"`
	CheckedAt     time.Time `json:"checked_at"`
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
}

type semanticVersion struct {
	core       [3]int
	prerelease string
}

func Default() (Checker, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return Checker{}, fmt.Errorf("locate user cache directory: %w", err)
	}
	return Checker{
		APIURL:    DefaultAPIURL,
		CachePath: filepath.Join(cacheRoot, "key-session", "update-check.json"),
		Client:    &http.Client{Timeout: 2 * time.Second},
		Now:       time.Now,
	}, nil
}

func (checker Checker) Check(context context.Context, currentVersion string, force bool) (Result, error) {
	if _, valid := parseVersion(currentVersion); !valid {
		return Result{}, fmt.Errorf("cannot check updates for non-release version %q", currentVersion)
	}
	if !force {
		if cached, err := checker.readCache(); err == nil {
			cacheAge := checker.Now().Sub(cached.CheckedAt)
			if cacheAge >= 0 && cacheAge < cacheLifetime {
				return resultFor(currentVersion, cached), nil
			}
		}
	}

	request, err := http.NewRequestWithContext(context, http.MethodGet, checker.APIURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("create update request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "key-session/"+currentVersion)
	response, err := checker.Client.Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("check GitHub releases: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("check GitHub releases: unexpected HTTP status %s", response.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&release); err != nil {
		return Result{}, fmt.Errorf("decode GitHub release: %w", err)
	}
	latestVersion := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if release.Draft || latestVersion == "" {
		return Result{}, fmt.Errorf("GitHub returned an invalid latest release")
	}
	if _, valid := parseVersion(latestVersion); !valid {
		return Result{}, fmt.Errorf("GitHub returned invalid version %q", release.TagName)
	}
	releaseURL := release.HTMLURL
	parsedReleaseURL, urlError := url.Parse(releaseURL)
	if urlError != nil || parsedReleaseURL.Scheme != "https" || parsedReleaseURL.Hostname() != "github.com" {
		releaseURL = DefaultReleasePage
	}
	cached := cachedRelease{LatestVersion: latestVersion, ReleaseURL: releaseURL, CheckedAt: checker.Now().UTC()}
	if err := checker.writeCache(cached); err != nil {
		return Result{}, err
	}
	return resultFor(currentVersion, cached), nil
}

func resultFor(currentVersion string, cached cachedRelease) Result {
	return Result{
		CurrentVersion:  currentVersion,
		LatestVersion:   cached.LatestVersion,
		UpdateAvailable: compareVersions(cached.LatestVersion, currentVersion) > 0,
		ReleaseURL:      cached.ReleaseURL,
		CheckedAt:       cached.CheckedAt,
	}
}

func (checker Checker) readCache() (cachedRelease, error) {
	contents, err := os.ReadFile(checker.CachePath)
	if err != nil {
		return cachedRelease{}, err
	}
	var cached cachedRelease
	if err := json.Unmarshal(contents, &cached); err != nil {
		return cachedRelease{}, err
	}
	if _, valid := parseVersion(cached.LatestVersion); !valid || cached.CheckedAt.IsZero() {
		return cachedRelease{}, fmt.Errorf("invalid update cache")
	}
	return cached, nil
}

func (checker Checker) writeCache(cached cachedRelease) error {
	directory := filepath.Dir(checker.CachePath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create update cache directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("protect update cache directory: %w", err)
	}
	contents, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("encode update cache: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".update-check-*")
	if err != nil {
		return fmt.Errorf("create update cache: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect update cache: %w", err)
	}
	if _, err := temporary.Write(append(contents, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write update cache: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close update cache: %w", err)
	}
	if err := os.Rename(temporaryPath, checker.CachePath); err != nil {
		return fmt.Errorf("install update cache: %w", err)
	}
	return nil
}

func parseVersion(value string) (semanticVersion, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if index := strings.IndexByte(value, '+'); index >= 0 {
		value = value[:index]
	}
	prerelease := ""
	if index := strings.IndexByte(value, '-'); index >= 0 {
		prerelease = value[index+1:]
		value = value[:index]
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	var parsed [3]int
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, false
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return semanticVersion{}, false
		}
		parsed[index] = number
	}
	if !validPrerelease(prerelease) {
		return semanticVersion{}, false
	}
	return semanticVersion{core: parsed, prerelease: prerelease}, true
}

func compareVersions(left string, right string) int {
	leftParts, leftValid := parseVersion(left)
	rightParts, rightValid := parseVersion(right)
	if !leftValid || !rightValid {
		return 0
	}
	for index := range leftParts.core {
		if leftParts.core[index] > rightParts.core[index] {
			return 1
		}
		if leftParts.core[index] < rightParts.core[index] {
			return -1
		}
	}
	return comparePrerelease(leftParts.prerelease, rightParts.prerelease)
}

func validPrerelease(prerelease string) bool {
	if prerelease == "" {
		return true
	}
	for _, identifier := range strings.Split(prerelease, ".") {
		if identifier == "" {
			return false
		}
		allDigits := true
		for _, character := range identifier {
			digit := character >= '0' && character <= '9'
			uppercase := character >= 'A' && character <= 'Z'
			lowercase := character >= 'a' && character <= 'z'
			if !digit && !uppercase && !lowercase && character != '-' {
				return false
			}
			if !digit {
				allDigits = false
			}
		}
		if allDigits && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}

func comparePrerelease(left string, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}
	leftIdentifiers := strings.Split(left, ".")
	rightIdentifiers := strings.Split(right, ".")
	for index := 0; index < len(leftIdentifiers) && index < len(rightIdentifiers); index++ {
		leftNumber, leftNumeric := numericIdentifier(leftIdentifiers[index])
		rightNumber, rightNumeric := numericIdentifier(rightIdentifiers[index])
		if leftNumeric && rightNumeric && leftNumber != rightNumber {
			if leftNumber > rightNumber {
				return 1
			}
			return -1
		}
		if leftNumeric != rightNumeric {
			if leftNumeric {
				return -1
			}
			return 1
		}
		if leftIdentifiers[index] != rightIdentifiers[index] {
			if leftIdentifiers[index] > rightIdentifiers[index] {
				return 1
			}
			return -1
		}
	}
	if len(leftIdentifiers) > len(rightIdentifiers) {
		return 1
	}
	return -1
}

func numericIdentifier(identifier string) (int, bool) {
	if identifier == "" || (len(identifier) > 1 && identifier[0] == '0') {
		return 0, false
	}
	number, err := strconv.Atoi(identifier)
	return number, err == nil
}
