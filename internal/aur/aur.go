package aur

import (
	"encoding/json"
	"fmt"

	"net/http"
	"net/url"
	"os/exec"
)

// Package represents a single AUR package result.
type aurPackage struct {
	Name        string  `json:"Name"`
	Version     string  `json:"Version"`
	Description string  `json:"Description"`
	URL         string  `json:"URL"`
	NumVotes    int     `json:"NumVotes"`
	Popularity  float64 `json:"Popularity"`
}

type rPCResponse struct {
	ResultCount int          `json:"resultcount"`
	Results     []aurPackage `json:"results"`
	Error       string       `json:"error"`
}

const aurRPCBase = "https://aur.archlinux.org/rpc/v5"

// Exists checks if a package with an exact name exists on the AUR.
func AURExists(pkgName string) (bool, *aurPackage, error) {
	apiURL := fmt.Sprintf("%s/info?arg[]=%s", aurRPCBase, url.QueryEscape(pkgName))

	resp, err := http.Get(apiURL)
	if err != nil {
		return false, nil, fmt.Errorf("failed to reach AUR API: %w", err)
	}
	defer resp.Body.Close()

	var result rPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, nil, fmt.Errorf("failed to parse AUR response: %w", err)
	}

	if result.Error != "" {
		return false, nil, fmt.Errorf("AUR API error: %s", result.Error)
	}

	// info endpoint returns exact matches only
	if result.ResultCount == 0 {
		return false, nil, nil
	}

	return true, &result.Results[0], nil
}

// Search does a fuzzy name search on AUR, returns up to 10 results.
func AURSearch(query string) ([]aurPackage, error) {
	apiURL := fmt.Sprintf("%s/search/%s?by=name", aurRPCBase, url.QueryEscape(query))

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to reach AUR API: %w", err)
	}
	defer resp.Body.Close()

	var result rPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse AUR response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("AUR API error: %s", result.Error)
	}

	return result.Results, nil
}

func IsInPacman(pkgName string) bool {
	cmd := exec.Command("pacman", "-Si", pkgName)
	cmd.Stdout = nil // suppress output, we only care about exit code
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// // Install clones the AUR package and builds it with makepkg.
// func Install(pkg *Package) error {
// 	cloneURL := fmt.Sprintf("https://aur.archlinux.org/%s.git", pkg.Name)

// 	// Use a temp dir for building
// 	buildDir := filepath.Join(os.TempDir(), "kubos-build", pkg.Name)
// 	if err := os.MkdirAll(buildDir, 0755); err != nil {
// 		return fmt.Errorf("failed to create build dir: %w", err)
// 	}
// 	defer os.RemoveAll(filepath.Join(os.TempDir(), "kubos-build"))

// 	fmt.Printf("==> [aur] cloning %s...\n", pkg.Name)
// 	cloneCmd := exec.Command("git", "clone", cloneURL, buildDir)
// 	cloneCmd.Stdout = os.Stdout
// 	cloneCmd.Stderr = os.Stderr
// 	if err := cloneCmd.Run(); err != nil {
// 		return fmt.Errorf("git clone failed: %w", err)
// 	}

// 	fmt.Printf("==> [aur] building %s with makepkg...\n", pkg.Name)
// 	// -s: install deps, -i: install after build, -c: clean up after
// 	buildCmd := exec.Command("makepkg", "-sic", "--noconfirm")
// 	buildCmd.Dir = buildDir
// 	buildCmd.Stdin = os.Stdin
// 	buildCmd.Stdout = os.Stdout
// 	buildCmd.Stderr = os.Stderr
// 	if err := buildCmd.Run(); err != nil {
// 		return fmt.Errorf("makepkg failed: %w", err)
// 	}

// 	fmt.Printf("==> [aur] %s installed successfully.\n", pkg.Name)
// 	return nil
// }
