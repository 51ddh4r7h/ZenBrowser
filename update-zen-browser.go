package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Configuration and constant definitions
const (
	githubAPIURL = "https://api.github.com/repos/zen-browser/desktop/releases/latest"
	coprProject  = "51ddh4r7h/zen-browser"
)

// ReleaseInfo stores the release information from GitHub
type ReleaseInfo struct {
	Version     string
	DownloadURL string
	Filename    string
	PublishedAt string
}

// GitHubRelease represents the GitHub release API response structure
type GitHubRelease struct {
	TagName     string  `json:"tag_name"`
	PublishedAt string  `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

// Asset represents a release asset from GitHub
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// Get the RPM build path, supporting different environments
func getRpmbuildPath() string {
	// First check if RPM_BUILD_ROOT environment variable is set
	if rpmBuildRoot, exists := os.LookupEnv("RPM_BUILD_ROOT"); exists {
		return rpmBuildRoot
	}

	// For GitHub Actions running in Fedora container
	if _, err := os.Stat("/root/rpmbuild"); err == nil {
		return "/root/rpmbuild"
	}

	// Default to user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(1)
	}
	return filepath.Join(homeDir, "rpmbuild")
}

// GetLatestRelease fetches the latest release information from GitHub
func getLatestRelease() (*ReleaseInfo, error) {
	resp, err := http.Get(githubAPIURL)
	if err != nil {
		return nil, fmt.Errorf("error accessing GitHub API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error accessing GitHub API: %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("error parsing GitHub API response: %v", err)
	}

	version := release.TagName

	// Skip twilight/nightly builds (containing 't' in version)
	if strings.Contains(version, "t") {
		fmt.Printf("Skipping twilight/nightly build version: %s\n", version)
		return nil, nil
	}

	// Find the Linux x86_64 asset
	var linuxAssetURL string
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, "linux-x86_64.tar.xz") {
			linuxAssetURL = asset.DownloadURL
			break
		}
	}

	if linuxAssetURL == "" {
		return nil, fmt.Errorf("could not find Linux x86_64 asset in the release")
	}

	return &ReleaseInfo{
		Version:     version,
		DownloadURL: fmt.Sprintf("https://github.com/zen-browser/desktop/releases/download/%s/zen.linux-x86_64.tar.xz", version),
		Filename:    "zen.linux-x86_64.tar.xz",
		PublishedAt: release.PublishedAt,
	}, nil
}

// UpdateSpecFile updates the spec file with the new version information
func updateSpecFile(specFilePath string, releaseInfo *ReleaseInfo) error {
	content, err := os.ReadFile(specFilePath)
	if err != nil {
		return fmt.Errorf("error reading spec file: %v", err)
	}

	// Update main version
	versionRegex := regexp.MustCompile(`Version:\s+.*`)
	updatedContent := versionRegex.ReplaceAllString(string(content), fmt.Sprintf("Version:        %s", releaseInfo.Version))

	// Update Source0 URL
	sourceURL := fmt.Sprintf("https://github.com/zen-browser/desktop/releases/download/%s/zen.linux-x86_64.tar.xz", releaseInfo.Version)
	sourceRegex := regexp.MustCompile(`Source0:\s+.*`)
	updatedContent = sourceRegex.ReplaceAllString(updatedContent, fmt.Sprintf("Source0:        %s", sourceURL))

	// Update desktop entry version
	desktopEntryRegex := regexp.MustCompile(`\[Desktop Entry\]\nVersion=.*`)
	updatedContent = desktopEntryRegex.ReplaceAllString(updatedContent, fmt.Sprintf("[Desktop Entry]\nVersion=%s", releaseInfo.Version))

	// Add new changelog entry
	today := time.Now().Format("Mon Jan 2 2006")
	changelogEntry := fmt.Sprintf("%%changelog\n* %s COPR Build System <copr-build@fedoraproject.org> - %s-1\n- Update to %s\n",
		today, releaseInfo.Version, releaseInfo.Version)
	changelogRegex := regexp.MustCompile(`%changelog.*`)
	updatedContent = changelogRegex.ReplaceAllString(updatedContent, changelogEntry)

	// Write the updated content back
	return os.WriteFile(specFilePath, []byte(updatedContent), 0644)
}

// DownloadSource downloads the source tarball
func downloadSource(sourcesDir, downloadURL, filename string) (string, error) {
	// Ensure the SOURCES directory exists
	if err := os.MkdirAll(sourcesDir, 0755); err != nil {
		return "", fmt.Errorf("error creating SOURCES directory: %v", err)
	}

	sourcePath := filepath.Join(sourcesDir, filename)

	// Download the file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("error downloading source: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error downloading source: %d", resp.StatusCode)
	}

	file, err := os.Create(sourcePath)
	if err != nil {
		return "", fmt.Errorf("error creating source file: %v", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("error saving source file: %v", err)
	}

	return sourcePath, nil
}

// BuildSRPM builds the SRPM package
func buildSRPM(specFilePath string) (string, error) {
	cmd := exec.Command("rpmbuild", "-bs", specFilePath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error building SRPM: %v\nStderr: %s", err, stderr.String())
	}

	// Try to find the SRPM path from the output
	srpmPath := findSRPMInOutput(stdout.String(), stderr.String())
	if srpmPath == "" {
		srpmPath = findSRPMInSpec(specFilePath)
	}
	if srpmPath == "" {
		srpmPath = findSRPMInDirectory(filepath.Join(filepath.Dir(filepath.Dir(specFilePath)), "SRPMS"))
	}

	if srpmPath == "" {
		return "", fmt.Errorf("could not find built SRPM path in output\nStdout: %s\nStderr: %s",
			stdout.String(), stderr.String())
	}

	fmt.Printf("Found SRPM: %s\n", srpmPath)
	return srpmPath, nil
}

// FindSRPMInOutput extracts SRPM path from command output
func findSRPMInOutput(stdout, stderr string) string {
	// First check stderr
	scanner := bufio.NewScanner(strings.NewReader(stderr))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasSuffix(line, ".src.rpm") {
			return strings.TrimPrefix(strings.TrimSpace(line), "Wrote: ")
		}
	}

	// Then check stdout
	scanner = bufio.NewScanner(strings.NewReader(stdout))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasSuffix(line, ".src.rpm") {
			return strings.TrimPrefix(strings.TrimSpace(line), "Wrote: ")
		}
	}

	return ""
}

// FindSRPMInSpec finds SRPM based on spec file version info
func findSRPMInSpec(specFilePath string) string {
	content, err := os.ReadFile(specFilePath)
	if err != nil {
		return ""
	}

	// Extract version
	versionRegex := regexp.MustCompile(`Version:\s+(.*)`)
	versionMatches := versionRegex.FindStringSubmatch(string(content))

	// Extract release
	releaseRegex := regexp.MustCompile(`Release:\s+(.*)`)
	releaseMatches := releaseRegex.FindStringSubmatch(string(content))

	if len(versionMatches) > 1 && len(releaseMatches) > 1 {
		version := versionMatches[1]
		release := strings.Replace(releaseMatches[1], "%{?dist}", ".fc41", 1)

		srpmDir := filepath.Join(filepath.Dir(filepath.Dir(specFilePath)), "SRPMS")
		expectedPath := filepath.Join(srpmDir, fmt.Sprintf("zen-browser-%s-%s.src.rpm", version, release))

		if _, err := os.Stat(expectedPath); err == nil {
			return expectedPath
		}
	}

	return ""
}

// FindSRPMInDirectory finds most recent SRPM in SRPMS directory
func findSRPMInDirectory(srpmsDir string) string {
	if err := os.MkdirAll(srpmsDir, 0755); err != nil {
		fmt.Printf("Error creating SRPMS directory: %v\n", err)
		return ""
	}

	files, err := os.ReadDir(srpmsDir)
	if err != nil {
		fmt.Printf("Error listing SRPMS directory: %v\n", err)
		return ""
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".src.rpm") {
			fmt.Printf(" - %s\n", file.Name())
			return filepath.Join(srpmsDir, file.Name())
		}
	}

	return ""
}

// SubmitToCopr submits the SRPM to COPR for building
func submitToCopr(srpmPath string) error {
	// Strip "Wrote: " prefix if present
	srpmPath = strings.TrimPrefix(srpmPath, "Wrote: ")

	fmt.Printf("Submitting %s to COPR project %s...\n", srpmPath, coprProject)

	cmd := exec.Command("copr-cli", "build", coprProject, srpmPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error submitting to COPR: %v\nStderr: %s", err, stderr.String())
	}

	fmt.Printf("Successfully submitted to COPR: %s\n", stdout.String())

	// Extract the build ID from the output
	buildIDRegex := regexp.MustCompile(`Created builds: (\d+)`)
	buildIDMatches := buildIDRegex.FindStringSubmatch(stdout.String())

	if len(buildIDMatches) > 1 {
		buildID := buildIDMatches[1]
		fmt.Printf("Build ID: %s\n", buildID)
		fmt.Printf("Build status URL: https://copr.fedorainfracloud.org/coprs/build/%s/\n", buildID)
	}

	return nil
}

func main() {
	fmt.Println("Checking for new Zen Browser releases...")

	// Set paths based on environment
	rpmbuildPath := getRpmbuildPath()
	specFilePath := filepath.Join(rpmbuildPath, "SPECS/zen-browser.spec")
	sourcesDir := filepath.Join(rpmbuildPath, "SOURCES")

	// Get latest release info
	releaseInfo, err := getLatestRelease()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Skip if we got nil due to twilight/nightly build
	if releaseInfo == nil {
		os.Exit(0)
	}

	// Check if this is a new version
	specContent, err := os.ReadFile(specFilePath)
	if err != nil {
		fmt.Printf("Error reading spec file: %v\n", err)
		os.Exit(1)
	}

	versionRegex := regexp.MustCompile(`Version:\s+(.*)`)
	versionMatches := versionRegex.FindStringSubmatch(string(specContent))

	if len(versionMatches) < 2 {
		fmt.Println("Error: Could not find Version in spec file")
		os.Exit(1)
	}

	currentVersion := versionMatches[1]

	if currentVersion == releaseInfo.Version {
		fmt.Printf("Already at the latest version: %s\n", currentVersion)
		return
	}

	fmt.Printf("New version found: %s\n", releaseInfo.Version)

	fmt.Println("Downloading source...")
	_, err = downloadSource(sourcesDir, releaseInfo.DownloadURL, releaseInfo.Filename)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Updating spec file...")
	err = updateSpecFile(specFilePath, releaseInfo)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Building SRPM...")
	srpmPath, err := buildSRPM(specFilePath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Submitting to COPR...")
	err = submitToCopr(srpmPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Done!")
}
