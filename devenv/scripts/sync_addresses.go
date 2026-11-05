package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var defaultEnvMapping = map[string]string{
	"staging_testnet_solana": "private-staging", // TODO: hmm just follow CLD env naming for devenv env id
	// "prod_testnet":           "prod-testnet",
}

func main() {
	cldPath := flag.String("cld-path", "", "Path to chainlink-deployments repository root (required)")
	env := flag.String("env", "", "Source deployment environment folder name (required)")
	devenvPath := flag.String("devenv-path", "devenv", "Path to devenv directory")
	targetFile := flag.String("target-file", "", "Explicit target TOML filename or full path (optional, overrides default naming)")
	flag.Parse()

	if *cldPath == "" || *env == "" {
		fmt.Println("Error: both -cld-path and -env flags are required")
		flag.Usage()
		os.Exit(1)
	}

	sourcePath := filepath.Clean(filepath.Join(*cldPath, "domains", "ccv", *env, "datastore", "address_refs.json"))

	// Resolve target TOML path
	var targetPath string
	if *targetFile != "" {
		if filepath.IsAbs(*targetFile) || strings.Contains(*targetFile, string(filepath.Separator)) {
			targetPath = filepath.Clean(*targetFile)
		} else {
			targetPath = filepath.Clean(filepath.Join(*devenvPath, *targetFile))
		}
	} else {
		targetEnv := defaultEnvMapping[*env]
		if targetEnv == "" {
			targetEnv = strings.ReplaceAll(*env, "_", "-")
		}
		targetPath = filepath.Clean(filepath.Join(*devenvPath, fmt.Sprintf("env-%s.deployed.toml", targetEnv)))
	}

	// 1. Read & compact JSON
	// #nosec G304 G703 -- CLI tool reading user-specified path
	jsonData, err := os.ReadFile(sourcePath)
	if err != nil {
		fmt.Printf("Error reading JSON (%s): %v\n", sourcePath, err)
		os.Exit(1)
	}

	var compactBuf bytes.Buffer
	if err = json.Compact(&compactBuf, jsonData); err != nil {
		fmt.Printf("Error compacting JSON: %v\n", err)
		os.Exit(1)
	}

	// 2. Decode TOML into dynamic map
	// #nosec G304 G703 -- CLI tool reading user-specified path
	tomlData, err := os.ReadFile(targetPath)
	if err != nil {
		fmt.Printf("Error reading TOML (%s): %v\n", targetPath, err)
		os.Exit(1)
	}

	var tomlTree map[string]any
	if err = toml.Unmarshal(tomlData, &tomlTree); err != nil {
		fmt.Printf("Error parsing TOML: %v\n", err)
		os.Exit(1)
	}

	// 3. Update [cldf].addresses array
	cldf, ok := tomlTree["cldf"].(map[string]any)
	if !ok {
		fmt.Println("Error: [cldf] table not found in TOML")
		os.Exit(1)
	}
	cldf["addresses"] = []string{compactBuf.String()}

	// 4. Marshal back to TOML
	output, err := toml.Marshal(tomlTree)
	if err != nil {
		fmt.Printf("Error encoding TOML: %v\n", err)
		os.Exit(1)
	}

	// #nosec G304 G703 -- CLI tool writing user-specified path
	if err := os.WriteFile(targetPath, output, 0600); err != nil {
		fmt.Printf("Error writing TOML (%s): %v\n", targetPath, err)
		os.Exit(1)
	}

	fmt.Printf("Successfully updated %s\n", targetPath)
}
