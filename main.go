package main

import (
	"bytes"
	"flag"
	"fmt"
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

	// Find the first regular file (not directory) and return its extension
	for _, entry := range entries {
		if !entry.IsDir() {
			return strings.TrimPrefix(filepath.Ext(entry.Name()), "."), nil
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

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
func cacheDir() string {
	if d, err := os.UserCacheDir(); err == nil && d != "" {
		return filepath.Join(d, "oscem-extractor")
	}
	// fallback to ~/.cache/oscem-extractor
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "oscem-extractor")
}

// findPackagedRoot walks up from start and returns the nearest ancestor
// directory that contains either a `dist` or `csv` subdirectory. If none
// is found, it returns the original start.
func findPackagedRoot(start string) string {
	dir := start
	for {
		distPath := filepath.Join(dir, "dist")
		csvPath := filepath.Join(dir, "csv")
		if dirExists(distPath) || dirExists(csvPath) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return start
}

func main() {
	inputDir := flag.String("i", "", "Input directory containing the file to process (required)")
	outputFile := flag.String("o", "", "Output file for results (required)")
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
	extractorEnv := *extractorFlag

	// asset name depends on OS
	extractorAssetName := "extractor_bin"
	if runtime.GOOS == "windows" {
		extractorAssetName = "extractor_bin.exe"
	}

	packagedRoot := findPackagedRoot(execDir)
	// if nothing packaged was found relative to the executable, also check the current working directory
	if !(dirExists(filepath.Join(packagedRoot, "dist")) || dirExists(filepath.Join(packagedRoot, "csv"))) {
		if cwd, err := os.Getwd(); err == nil {
			pw := findPackagedRoot(cwd)
			if dirExists(filepath.Join(pw, "dist")) || dirExists(filepath.Join(pw, "csv")) {
				packagedRoot = pw
			}
		}
	}
	packagedExtractor := filepath.Join(packagedRoot, "dist", extractorAssetName)

	var extractorPath string
	if extractorEnv != "" {
		extractorPath = extractorEnv
	} else if fileExists(packagedExtractor) {
		extractorPath = packagedExtractor
	} else {
		fmt.Fprintln(os.Stderr, "Extractor not provided.")
		os.Exit(1)
	}

	fmt.Println("=== Running Python extractor ===")
	args := []string{*inputDir}
	data, err := runCmd(extractorPath, args...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Extractor failed due to:", err)
		os.Exit(1)
	}

	fmt.Println("=== Running Go converter ===")
	// Resolve CSV path
	csvAssetName := "ms_conversions_" + fileExt + ".csv"
	// packagedRoot already computed above (execDir or cwd)
	packagedCSV := filepath.Join(packagedRoot, "csv", csvAssetName)

	var converterCSVPath string
	if fileExists(packagedCSV) {
		converterCSVPath = packagedCSV
	} else {
		fmt.Fprintln(os.Stderr, "CSV not provided.")
		os.Exit(1)
	}

	out, err := conversion.Convert([]byte(data), converterCSVPath, "", "", outputFilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Converter failed due to:", err)
		os.Exit(1)
	}

	fmt.Println("\n=== MS Reader results ===")
	fmt.Println(string(out))
}
