package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Only the network boundary changes: parsing, version ordering and installer
// selection run against the same release shape returned by GitHub.
type releaseTestTransport struct {
	server *httptest.Server
}

func (r releaseTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme = "http"
	copy.URL.Host = strings.TrimPrefix(r.server.URL, "http://")
	return http.DefaultTransport.RoundTrip(copy)
}

func releaseCheckClient(t *testing.T, status int, body string) *http.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/OUBIGFA/OneSub/releases/latest" || r.Header.Get("User-Agent") == "" {
			t.Errorf("wrong public release request: %s", r.URL)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return &http.Client{Transport: releaseTestTransport{server: server}}
}

const installerOnlyRelease = `{
	"tag_name":"0.1.3", "draft":false, "prerelease":false,
	"assets":[{"name":"OneSub-0.1.3-Setup.exe", "state":"uploaded", "size":201662370,
	"browser_download_url":"https://github.com/OUBIGFA/OneSub/releases/download/0.1.3/OneSub-0.1.3-Setup.exe"}]
}`

func TestInstallerReleaseDetectsPublishedVersionWithoutSignedArchive(t *testing.T) {
	client := releaseCheckClient(t, http.StatusOK, installerOnlyRelease)
	installer, err := checkInstallerRelease(context.Background(), client, "0.1.2")
	if err != nil || installer == nil {
		t.Fatalf("published 0.1.3 installer was not offered to 0.1.2: %v, %v", installer, err)
	}
	if installer.Version != "0.1.3" || installer.URL != "https://github.com/OUBIGFA/OneSub/releases/download/0.1.3/OneSub-0.1.3-Setup.exe" {
		t.Fatalf("wrong installer: %+v", installer)
	}
}

func TestInstallerReleaseComparesProductVersionsNumerically(t *testing.T) {
	for _, tc := range []struct {
		current, published string
		available          bool
	}{
		{"0.1.3", "0.1.3", false},
		{"0.1.4", "0.1.3", false},
		{"0.1.9", "0.1.10", true},
		{"0.2.0", "0.1.10", false},
		{"1.0.0", "2.0.0", true},
	} {
		t.Run(tc.current+"/"+tc.published, func(t *testing.T) {
			body := strings.ReplaceAll(installerOnlyRelease, "0.1.3", tc.published)
			installer, err := checkInstallerRelease(context.Background(), releaseCheckClient(t, http.StatusOK, body), tc.current)
			if err != nil || (installer != nil) != tc.available {
				t.Fatalf("installer=%+v, err=%v, want available=%v", installer, err, tc.available)
			}
		})
	}
}

func TestInstallerReleaseDoesNotOfferUnpublishedOrUnrelatedDownloads(t *testing.T) {
	for name, body := range map[string]string{
		"draft":          strings.Replace(installerOnlyRelease, `"draft":false`, `"draft":true`, 1),
		"prerelease":     strings.Replace(installerOnlyRelease, `"prerelease":false`, `"prerelease":true`, 1),
		"missing asset":  `{"tag_name":"0.1.3","assets":[]}`,
		"wrong asset":    strings.ReplaceAll(installerOnlyRelease, "Setup.exe", "arm64.exe"),
		"not uploaded":   strings.ReplaceAll(installerOnlyRelease, "uploaded", "new"),
		"empty download": strings.ReplaceAll(installerOnlyRelease, "201662370", "0"),
		"foreign URL":    strings.ReplaceAll(installerOnlyRelease, "https://github.com/", "https://other.invalid/"),
	} {
		t.Run(name, func(t *testing.T) {
			installer, err := checkInstallerRelease(context.Background(), releaseCheckClient(t, http.StatusOK, body), "0.1.2")
			if installer != nil || !errors.Is(err, errNoPublishedInstaller) {
				t.Fatalf("unavailable installer was accepted: %+v, %v", installer, err)
			}
		})
	}
}

func TestInstallerReleaseDistinguishesAbsenceFailureAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		absent bool
	}{
		{"no release", http.StatusNotFound, `{}`, true},
		{"rate limit", http.StatusForbidden, `{}`, false},
		{"server failure", http.StatusBadGateway, `{}`, false},
		{"invalid JSON", http.StatusOK, `{`, false},
		{"empty release", http.StatusOK, `{}`, false},
		{"invalid version", http.StatusOK, strings.ReplaceAll(installerOnlyRelease, "0.1.3", "not-a-version"), false},
		{"oversized response", http.StatusOK, installerOnlyRelease + strings.Repeat(" ", 1<<20), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installer, err := checkInstallerRelease(context.Background(), releaseCheckClient(t, tc.status, tc.body), "0.1.2")
			if installer != nil || err == nil || errors.Is(err, errNoPublishedInstaller) != tc.absent {
				t.Fatalf("wrong failure classification: %+v, %v", installer, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	installer, err := checkInstallerRelease(ctx, releaseCheckClient(t, http.StatusOK, installerOnlyRelease), "0.1.2")
	if installer != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled check returned %+v, %v", installer, err)
	}
}

func TestInstallerReleaseStatusOffersManualDownloadWithoutAutomaticInstall(t *testing.T) {
	// This state-only regression does not create a native window or touch an
	// installed app. An installer must never enter MyGo's automatic path.
	a := &NativeApp{}
	a.updates.enabled = true
	a.settings.AutoUpdate = true
	installer := &installerRelease{Version: "0.1.3", URL: "https://github.com/OUBIGFA/OneSub/releases/download/0.1.3/OneSub-0.1.3-Setup.exe"}
	a.completeInstallerCheck(installer, nil)
	if a.updates.phase != updateInstallerAvailable || a.updates.installer != installer || a.updates.release != nil || a.automaticUpdateReady() {
		t.Fatalf("manual installer entered automatic installation: %+v", a.updates)
	}
	if a.updateStatus() != locales.AboutInstallerAvailable("0.1.3") {
		t.Fatal("status does not identify the available installer", a.updateStatus())
	}
	for _, tc := range []struct {
		err   error
		phase updatePhase
	}{
		{nil, updateCurrent},
		{errNoPublishedInstaller, updateMissing},
		{errors.New("rate limited"), updateFailed},
		{context.Canceled, updateCancelled},
	} {
		a.completeInstallerCheck(installer, nil)
		a.completeInstallerCheck(nil, tc.err)
		if a.updates.phase != tc.phase || a.updates.installer != nil || a.updates.release != nil {
			t.Fatalf("stale installer survived a new result: %+v", a.updates)
		}
	}
}

func TestInstallerReleaseFallbackOnlyUsesMissingFeed(t *testing.T) {
	for _, tc := range []struct {
		err     error
		missing bool
	}{
		{errors.New("mygo: checking for updates: GET https://github.com/OUBIGFA/OneSub/releases/latest/download/update-windows-amd64.json: 404 Not Found"), true},
		{nil, false},
		{context.Canceled, false},
		{errors.New("403 Forbidden"), false},
		{errors.New("invalid signature"), false},
	} {
		if missingUpdateFeed(tc.err) != tc.missing {
			t.Fatalf("wrong installer fallback for %v", tc.err)
		}
	}
}
