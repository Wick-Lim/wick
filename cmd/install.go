package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

// PackageMetadata 구조체 정의
type PackageMetadata struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
	Dist         struct {
		Tarball string `json:"tarball"`
	} `json:"dist"`
}

// 임시 구조체 정의
type RawMetadata struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Dependencies map[string]interface{} `json:"dependencies"`
	Dist         map[string]interface{} `json:"dist"`
}

func fetchMetadata(packageName, version string) (*PackageMetadata, error) {
	if version == "latest" {
		var err error
		version, err = getLatestVersion(packageName)
		if err != nil {
			return nil, err
		}
	}

	url := fmt.Sprintf("https://registry.npmjs.org/%s/%s", packageName, version)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 임시 구조체에 JSON 데이터를 매핑
	var raw RawMetadata
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	// 필드 검증 및 변환
	if _, ok := raw.Dist["tarball"].(string); !ok {
		return nil, errors.New("invalid package metadata: tarball URL missing")
	}

	dependencies := make(map[string]string)
	for k, v := range raw.Dependencies {
		if versionStr, ok := v.(string); ok {
			dependencies[k] = versionStr
		}
	}

	metadata := &PackageMetadata{
		Name:         raw.Name,
		Version:      raw.Version,
		Dependencies: dependencies,
		Dist: struct {
			Tarball string `json:"tarball"`
		}{
			Tarball: raw.Dist["tarball"].(string),
		},
	}

	return metadata, nil
}

// 최신 버전을 조회하는 함수
func getLatestVersion(packageName string) (string, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s", packageName)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	// "dist-tags"에서 "latest" 버전을 확인
	latestVersion, ok := data["dist-tags"].(map[string]interface{})["latest"].(string)
	if !ok {
		return "", fmt.Errorf("could not find latest version for package %s", packageName)
	}
	return latestVersion, nil
}

func resolveDependencies(packageName, version string, resolved map[string]bool) error {
	if resolved[packageName+"@"+version] {
		return nil
	}

	metadata, err := fetchMetadata(packageName, version)
	if err != nil {
		return err
	}

	for depName, depVersion := range metadata.Dependencies {
		if err := resolveDependencies(depName, depVersion, resolved); err != nil {
			return err
		}
	}

	fmt.Printf("Resolved %s@%s\n", packageName, version)
	resolved[packageName+"@"+version] = true

	// 패키지 다운로드 로직 (예시)
	fmt.Printf("Downloading %s...\n", metadata.Dist.Tarball)

	return nil
}

// installCmd 명령어 정의
var installCmd = &cobra.Command{
	Use:   "install [package-name] [version]",
	Short: "Install a package with the specified name and version",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		packageName := args[0]
		version := "latest"
		if len(args) == 2 {
			version = args[1]
		}

		resolved := make(map[string]bool)
		if err := resolveDependencies(packageName, version, resolved); err != nil {
			fmt.Println("Error resolving dependencies:", err)
			os.Exit(1)
		}

		fmt.Println("All dependencies resolved and installed.")
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
