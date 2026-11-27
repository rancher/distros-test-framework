package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// GitHubAPI is the API base used to resolve KDM pull requests to their head commit.
const GitHubAPI = "https://api.github.com"

const maxMetadataBytes = 64 << 20

var (
	prURLPattern = regexp.MustCompile(`^https://github\.com/([\w.-]+/[\w.-]+)/pull/(\d+)/?$`)
	// Rejects anything a remote shell could misread: versions are interpolated into install commands.
	versionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(-rc\d+)?\+(k3s|rke2r)\d+$`)
)

// Release is one k3s/rke2 entry of a KDM data.json.
type Release struct {
	Version                 string `json:"version"`
	MinChannelServerVersion string `json:"minChannelServerVersion"`
	MaxChannelServerVersion string `json:"maxChannelServerVersion"`
}

type metadata struct {
	K3s  struct{ Releases []Release } `json:"k3s"`
	RKE2 struct{ Releases []Release } `json:"rke2"`
}

// ValidateVersion checks a k3s/rke2 version string belongs to product and is safe to pass to a shell.
func ValidateVersion(product, version string) error {
	m := versionPattern.FindStringSubmatch(version)
	if m == nil {
		return fmt.Errorf("version %q does not look like v1.2.3[-rcN]+k3sN or +rke2rN", version)
	}
	if (product == "k3s") != (m[2] == "k3s") {
		return fmt.Errorf("version %q does not match product %q", version, product)
	}

	return nil
}

// ResolveDataURL accepts a raw data.json URL or a GitHub PR URL and returns
// a data.json URL pinned to the PR head commit, so a later push can't change the run.
func ResolveDataURL(ctx context.Context, apiBase, source string) (string, error) {
	source = strings.TrimSpace(source)
	m := prURLPattern.FindStringSubmatch(source)
	if m == nil {
		u, err := url.Parse(source)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return "", fmt.Errorf("KDM source must be an https data.json URL or a GitHub PR URL, got %q", source)
		}

		return source, nil
	}

	var pr struct {
		Head struct {
			SHA  string `json:"sha"`
			Repo *struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
	}
	if err := getJSON(ctx, http.DefaultClient, apiBase+"/repos/"+m[1]+"/pulls/"+m[2], &pr); err != nil {
		return "", fmt.Errorf("resolve KDM PR %s: %w", source, err)
	}
	if pr.Head.Repo == nil || pr.Head.SHA == "" {
		return "", fmt.Errorf("KDM PR %s has no head repository (fork deleted?)", source)
	}

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/data/data.json", pr.Head.Repo.FullName, pr.Head.SHA), nil
}

// FetchReleases downloads data.json and returns the releases listed for product.
func FetchReleases(ctx context.Context, dataURL, product string) ([]Release, error) {
	var md metadata
	if err := getJSON(ctx, http.DefaultClient, dataURL, &md); err != nil {
		return nil, fmt.Errorf("fetch KDM data %s: %w", dataURL, err)
	}

	switch product {
	case "k3s":
		return md.K3s.Releases, nil
	case "rke2":
		return md.RKE2.Releases, nil
	default:
		return nil, fmt.Errorf("unsupported product %q", product)
	}
}

// FindRelease returns the release matching version exactly.
func FindRelease(releases []Release, version string) (Release, bool) {
	for _, r := range releases {
		if r.Version == version {
			return r, true
		}
	}

	return Release{}, false
}

func getJSON(ctx context.Context, client *http.Client, target string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d: %s", target, resp.StatusCode, truncate(body))
	}

	return json.Unmarshal(body, out)
}

func truncate(b []byte) string {
	const limit = 300
	if len(b) > limit {
		return string(b[:limit]) + "..."
	}

	return string(b)
}

// VersionUnderTest returns the RC the run tests: the host cluster's INSTALL_VERSION. An explicit
// k8sVersion must match it, so the host, custom and upgraded clusters always run the same RC.
func VersionUnderTest(installVersion, k8sVersion string) (string, error) {
	if k8sVersion == "" || k8sVersion == installVersion {
		return installVersion, nil
	}

	return "", fmt.Errorf("k8sVersion %q differs from INSTALL_VERSION %q: the host cluster must run the version under test",
		k8sVersion, installVersion)
}

var rcSuffix = regexp.MustCompile(`-rc\d+`)

// ReportedVersion is how kubelets and Rancher report a version: RC builds drop the -rcN suffix.
func ReportedVersion(version string) string {
	return rcSuffix.ReplaceAllString(version, "")
}
