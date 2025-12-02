package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func main() {
	inputDir := flag.String("i", "", "Input directory containing the file to process (required)")
	outputFile := flag.String("o", "", "Output file for results (required)")

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
	extractorPath := filepath.Join(execDir, "dist", "extractor_bin")

	fmt.Println("=== Running Python extractor ===")
	args := []string{*inputDir}
	data, err := runCmd(extractorPath, args...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Extractor failed due to:", err)
		os.Exit(1)
	}

	fmt.Println("=== Running Go converter ===")
	// Use the CSV file downloaded by go generate in csv folder
	converterCSVPath := filepath.Join(execDir, "csv", "ms_conversions_"+fileExt+".csv")
	out, err := conversion.Convert([]byte(data), converterCSVPath, "", "", outputFilePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Converter failed due to:", err)
		os.Exit(1)
	}

	fmt.Println("\n=== MS Reader results ===")
	fmt.Println(string(out))
}
