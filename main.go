package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	conversion "github.com/osc-em/oscem-converter-extracted"
)

//go:generate mkdir -p csv
//go:generate wget https://raw.githubusercontent.com/osc-em/oscem-converter-extracted/refs/heads/main/csv/ms_conversions_emd.csv -O csv/ms_conversions_emd.csv
//go:generate wget https://raw.githubusercontent.com/osc-em/oscem-converter-extracted/refs/heads/main/csv/ms_conversions_prz.csv -O csv/ms_conversions_prz.csv

func getFileTypeFromDir(dirPath string) (string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return "", err
	}

	// Find the first regular file (not directory)
	for _, entry := range entries {
		if !entry.IsDir() {
			filename := entry.Name()
			ext := strings.ToLower(filepath.Ext(filename))
			return strings.TrimPrefix(ext, "."), nil
		}
	}
	return "", fmt.Errorf("no regular file found in directory")
}

func runCmd(name string, args ...string) (string, error) {
	var outBuf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return outBuf.String(), err
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func downloadURLToFile(url, dest string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	if err := os.Chmod(dest, 0o755); err != nil {
		return err
	}
	return nil
}

func buildReleaseDownloadURL(tag, assetName string) string {
	return fmt.Sprintf("https://github.com/osc-em/oscem-converter-extracted/releases/download/%s/%s", tag, assetName)
}

// cacheDir returns a cache directory for downloaded assets.
func cacheDir() string {
	if d, err := os.UserCacheDir(); err == nil && d != "" {
		return filepath.Join(d, "oscem-extractor")
	}
	// fallback to ~/.cache/oscem-extractor
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "oscem-extractor")
}

func main() {
	inputDir := flag.String("i", "", "Input directory containing the file to process (required)")
	outputFile := flag.String("o", "", "Output file for results (required)")
	tagFlag := flag.String("t", "", "GitHub release tag to download assets from (required)")
	extractorFlag := flag.String("e", "", "Path to local extractor binary (optional, developer override)")

	flag.Parse()
	if *inputDir == "" || *outputFile == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -i <input_directory> -o <output_file>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Get file type from input directory
	fileExt, err := getFileTypeFromDir(*inputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to detect file type:", err)
		os.Exit(1)
	}

	// Ensure output file directory exists
	outputFilePath := *outputFile
	if err := os.MkdirAll(filepath.Dir(outputFilePath), 0755); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create output file directory:", err)
		os.Exit(1)
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to get executable path:", err)
		os.Exit(1)
	}
	execDir := filepath.Dir(execPath)

	// Resolve extractor binary path. Priority:
	// 1. Explicit local override of extractor binary (for development)
	// 2. Packaged file inside the distribution (execDir/dist/...)
	// 3. GitHub release asset identified by githubTag
	extractorEnv := *extractorFlag
	githubTag := *tagFlag

	// asset name depends on OS
	extractorAssetName := "extractor_bin"
	if runtime.GOOS == "windows" {
		extractorAssetName = "extractor_bin.exe"
	}

	cache := cacheDir()
	cachedExtractor := filepath.Join(cache, "dist", extractorAssetName)
	packagedExtractor := filepath.Join(execDir, "dist", extractorAssetName)

	var extractorPath string
	if extractorEnv != "" {
		extractorPath = extractorEnv
	} else if fileExists(packagedExtractor) {
		extractorPath = packagedExtractor
	} else {
		if githubTag == "" {
			fmt.Fprintln(os.Stderr, "Extractor not provided. Provide GitHub release tag or set local extractor path (for development).")
			os.Exit(1)
		}
		if !fileExists(cachedExtractor) {
			url := buildReleaseDownloadURL(githubTag, extractorAssetName)
			if err := downloadURLToFile(url, cachedExtractor); err != nil {
				fmt.Fprintln(os.Stderr, "Failed to download extractor asset:", err)
				os.Exit(1)
			}
		}
		extractorPath = cachedExtractor
	}

	fmt.Println("=== Running Python extractor ===")
	args := []string{*inputDir}
	data, err := runCmd(extractorPath, args...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Extractor failed due to:", err)
		os.Exit(1)
	}

	fmt.Println("=== Running Go converter ===")
	// Resolve CSV path. Priority:
	// 1. Packaged csv under execDir/csv/
	// 2. GitHub release asset `ms_conversions_<ext>.csv`
	csvAssetName := "ms_conversions_" + fileExt + ".csv"
	packagedCSV := filepath.Join(execDir, "csv", csvAssetName)
	cachedCSV := filepath.Join(cache, "csv", csvAssetName)

	var converterCSVPath string
	if fileExists(packagedCSV) {
		converterCSVPath = packagedCSV
	} else {
		if githubTag == "" {
			fmt.Fprintln(os.Stderr, "CSV not provided. Provide GitHub release tag.")
			os.Exit(1)
		}
		if !fileExists(cachedCSV) {
			url := buildReleaseDownloadURL(githubTag, csvAssetName)
			if err := downloadURLToFile(url, cachedCSV); err != nil {
				fmt.Fprintln(os.Stderr, "Failed to download CSV asset:", err)
				os.Exit(1)
			}
		}
		converterCSVPath = cachedCSV
	}

	out, err := conversion.Convert([]byte(data), converterCSVPath, "", "", outputFilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Converter failed due to:", err)
		os.Exit(1)
	}

	fmt.Println("\n=== MS Reader results ===")
	fmt.Println(string(out))
}
