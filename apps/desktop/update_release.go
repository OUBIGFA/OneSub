package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var errNoPublishedInstaller = errors.New("no published installer is available")

// An installer-only release is a browser download, never a signed MyGo
// update. Keep the types separate so automatic installation cannot use it.
type installerRelease struct {
	Version string
	URL     string
}

// checkInstallerRelease covers the public installer-only release policy when
// MyGo's signed feed is absent. Only the product version is comparable here;
// GitHub does not publish the internal build number of an installer.
func checkInstallerRelease(ctx context.Context, client *http.Client, current string) (*installerRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/OUBIGFA/OneSub/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "OneSub-updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errNoPublishedInstaller
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checking published installer: %s", resp.Status)
	}
	const maxReleaseBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxReleaseBytes {
		return nil, errors.New("published release response is too large")
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name  string `json:"name"`
			State string `json:"state"`
			Size  int64  `json:"size"`
			URL   string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("reading published release: %w", err)
	}
	if release.Draft || release.Prerelease {
		return nil, errNoPublishedInstaller
	}
	version := strings.TrimPrefix(release.Tag, "v")
	published, err := parseProductVersion(version)
	if err != nil {
		return nil, err
	}
	installed, err := parseProductVersion(current)
	if err != nil {
		return nil, err
	}
	newer := false
	for i := range published {
		if published[i] != installed[i] {
			newer = published[i] > installed[i]
			break
		}
	}
	if !newer {
		return nil, nil
	}
	name := "OneSub-" + version + "-Setup.exe"
	url := repositoryURL + "/releases/download/" + release.Tag + "/" + name
	for _, asset := range release.Assets {
		if asset.Name == name && asset.State == "uploaded" && asset.Size > 0 && asset.URL == url {
			return &installerRelease{Version: version, URL: url}, nil
		}
	}
	return nil, errNoPublishedInstaller
}

// Public product tags have exactly three numeric components. Do not compare
// them as text (0.1.10 sorts before 0.1.9) or accept an unrecognized tag as 0.
func parseProductVersion(version string) ([3]uint64, error) {
	var result [3]uint64
	parts := strings.Split(version, ".")
	if len(parts) != len(result) {
		return result, fmt.Errorf("invalid product version %q", version)
	}
	for i, part := range parts {
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil || strconv.FormatUint(value, 10) != part {
			return result, fmt.Errorf("invalid product version %q", version)
		}
		result[i] = value
	}
	return result, nil
}
