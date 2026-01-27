package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ForgeType represents a git forge type
type ForgeType string

const (
	ForgeGitHub  ForgeType = "github"
	ForgeGitLab  ForgeType = "gitlab"
	ForgeGitea   ForgeType = "gitea"
	ForgeForgejo ForgeType = "forgejo"
	ForgeUnknown ForgeType = "unknown"
)

// Forge provides release operations for git forges
type Forge struct {
	host      string
	repo      string
	forgeType ForgeType
	token     string
	client    *http.Client
}

// NewForge creates a new Forge instance
func NewForge(host, repo, token string) *Forge {
	return &Forge{
		host:  host,
		repo:  repo,
		token: token,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// DetectType detects the forge type from the host
func (f *Forge) DetectType() (ForgeType, error) {
	// Known hosts
	switch f.host {
	case "github.com":
		f.forgeType = ForgeGitHub
		return f.forgeType, nil
	case "gitlab.com":
		f.forgeType = ForgeGitLab
		return f.forgeType, nil
	case "codeberg.org":
		f.forgeType = ForgeForgejo
		return f.forgeType, nil
	case "gitea.com":
		f.forgeType = ForgeGitea
		return f.forgeType, nil
	}

	// Probe APIs for unknown hosts
	if f.probeAPI("/api/v4/version") {
		f.forgeType = ForgeGitLab
		return f.forgeType, nil
	}

	if f.probeAPI("/api/v1/version") {
		// Could be Gitea or Forgejo - check response
		if f.isForgejo() {
			f.forgeType = ForgeForgejo
		} else {
			f.forgeType = ForgeGitea
		}
		return f.forgeType, nil
	}

	if f.probeAPI("/api/v3/meta") {
		f.forgeType = ForgeGitHub
		return f.forgeType, nil
	}

	f.forgeType = ForgeUnknown
	return f.forgeType, fmt.Errorf("could not detect forge type for %s", f.host)
}

func (f *Forge) probeAPI(path string) bool {
	req, err := http.NewRequest("GET", "https://"+f.host+path, nil)
	if err != nil {
		return false
	}

	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func (f *Forge) isForgejo() bool {
	req, err := http.NewRequest("GET", "https://"+f.host+"/api/v1/version", nil)
	if err != nil {
		return false
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return strings.Contains(strings.ToLower(string(body)), "forgejo")
}

// CreateRelease creates a release on the forge
func (f *Forge) CreateRelease(tag, changelog string) (string, error) {
	switch f.forgeType {
	case ForgeGitHub:
		return f.createGitHubRelease(tag, changelog)
	case ForgeGitLab:
		return f.createGitLabRelease(tag, changelog)
	case ForgeGitea, ForgeForgejo:
		return f.createGiteaRelease(tag, changelog)
	default:
		return "", fmt.Errorf("unsupported forge type: %s", f.forgeType)
	}
}

// UploadAsset uploads an asset to the release
func (f *Forge) UploadAsset(releaseID, filePath string) error {
	switch f.forgeType {
	case ForgeGitHub:
		return f.uploadGitHubAsset(releaseID, filePath)
	case ForgeGitLab:
		return f.uploadGitLabAsset(releaseID, filePath)
	case ForgeGitea, ForgeForgejo:
		return f.uploadGiteaAsset(releaseID, filePath)
	default:
		return fmt.Errorf("unsupported forge type: %s", f.forgeType)
	}
}

// GitHub implementation
func (f *Forge) createGitHubRelease(tag, changelog string) (string, error) {
	apiURL := "https://api.github.com"
	if f.host != "github.com" {
		apiURL = "https://" + f.host + "/api/v3"
	}

	payload := map[string]interface{}{
		"tag_name":   tag,
		"name":       tag,
		"body":       changelog,
		"draft":      false,
		"prerelease": false,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", apiURL+"/repos/"+f.repo+"/releases", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to create release: %s", string(respBody))
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	return fmt.Sprintf("%d", result.ID), nil
}

func (f *Forge) uploadGitHubAsset(releaseID, filePath string) error {
	fileName := filepath.Base(filePath)

	uploadURL := "https://uploads.github.com"
	if f.host != "github.com" {
		uploadURL = "https://" + f.host + "/api/uploads"
	}
	uploadURL += fmt.Sprintf("/repos/%s/releases/%s/assets?name=%s", f.repo, releaseID, url.QueryEscape(fileName))

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	req, err := http.NewRequest("POST", uploadURL, file)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/gzip")

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload asset: %s", string(body))
	}

	return nil
}

// GitLab implementation
func (f *Forge) createGitLabRelease(tag, changelog string) (string, error) {
	apiURL := "https://" + f.host + "/api/v4"
	encodedRepo := url.PathEscape(f.repo)

	payload := map[string]interface{}{
		"tag_name":    tag,
		"name":        tag,
		"description": changelog,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", apiURL+"/projects/"+encodedRepo+"/releases", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("PRIVATE-TOKEN", f.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to create release: %s", string(respBody))
	}

	return tag, nil // GitLab uses tag as release ID
}

func (f *Forge) uploadGitLabAsset(tag, filePath string) error {
	apiURL := "https://" + f.host + "/api/v4"
	encodedRepo := url.PathEscape(f.repo)
	fileName := filepath.Base(filePath)

	// Upload to Generic Package Registry
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	uploadURL := fmt.Sprintf("%s/projects/%s/packages/generic/plasma-release/%s/%s",
		apiURL, encodedRepo, tag, url.PathEscape(fileName))

	req, err := http.NewRequest("PUT", uploadURL, file)
	if err != nil {
		return err
	}

	req.Header.Set("PRIVATE-TOKEN", f.token)

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload asset: %s", string(body))
	}

	// Link asset to release
	downloadURL := fmt.Sprintf("%s/projects/%s/packages/generic/plasma-release/%s/%s",
		apiURL, encodedRepo, tag, url.PathEscape(fileName))

	linkPayload := map[string]interface{}{
		"name":      fileName,
		"url":       downloadURL,
		"link_type": "package",
	}

	linkBody, _ := json.Marshal(linkPayload)
	linkReq, err := http.NewRequest("POST",
		fmt.Sprintf("%s/projects/%s/releases/%s/assets/links", apiURL, encodedRepo, tag),
		bytes.NewReader(linkBody))
	if err != nil {
		return err
	}

	linkReq.Header.Set("PRIVATE-TOKEN", f.token)
	linkReq.Header.Set("Content-Type", "application/json")

	linkResp, err := f.client.Do(linkReq)
	if err != nil {
		return err
	}
	defer linkResp.Body.Close()

	return nil
}

// Gitea/Forgejo implementation
func (f *Forge) createGiteaRelease(tag, changelog string) (string, error) {
	apiURL := "https://" + f.host + "/api/v1"

	payload := map[string]interface{}{
		"tag_name":   tag,
		"name":       tag,
		"body":       changelog,
		"draft":      false,
		"prerelease": false,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", apiURL+"/repos/"+f.repo+"/releases", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "token "+f.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to create release: %s", string(respBody))
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	return fmt.Sprintf("%d", result.ID), nil
}

func (f *Forge) uploadGiteaAsset(releaseID, filePath string) error {
	apiURL := "https://" + f.host + "/api/v1"
	fileName := filepath.Base(filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	boundary := "----PlasmaReleaseBoundary"

	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"attachment\"; filename=\"%s\"\r\n", fileName))
	buf.WriteString("Content-Type: application/octet-stream\r\n\r\n")

	fileContent, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	buf.Write(fileContent)
	buf.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))

	uploadURL := fmt.Sprintf("%s/repos/%s/releases/%s/assets?name=%s",
		apiURL, f.repo, releaseID, url.QueryEscape(fileName))

	req, err := http.NewRequest("POST", uploadURL, &buf)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "token "+f.token)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upload asset: %s", string(body))
	}

	return nil
}

// ResolveToken resolves a token from argument or environment variables
func ResolveToken(argToken string, forgeType ForgeType) string {
	if argToken != "" {
		return argToken
	}

	switch forgeType {
	case ForgeGitHub:
		return os.Getenv("GITHUB_TOKEN")
	case ForgeGitLab:
		return os.Getenv("GITLAB_TOKEN")
	case ForgeGitea, ForgeForgejo:
		return os.Getenv("GITEA_TOKEN")
	}

	return ""
}
