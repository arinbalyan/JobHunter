// Command: syncsecrets
// Reads .env and syncs all values to GitHub repository secrets via gh CLI.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// skipSecrets are env vars that should never be pushed to GitHub secrets.
var skipSecrets = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "SHELL": true,
	"PWD": true, "OLDPWD": true, "TERM": true, "LANG": true,
	"LOGNAME": true, "EDITOR": true, "GOPATH": true,
}

func main() {
	fmt.Println("  JobHunter Secret Sync")
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Println()

	// Check gh CLI
	if _, err := exec.LookPath("gh"); err != nil {
		fmt.Println("  \033[31m✗ GitHub CLI (gh) not found in PATH.\033[0m")
		fmt.Println("  Install from: https://cli.github.com/")
		fmt.Println("  Then authenticate: gh auth login")
		os.Exit(1)
	}

	// Check gh auth
	authCheck := exec.Command("gh", "auth", "status")
	if err := authCheck.Run(); err != nil {
		fmt.Println("  \033[31m✗ Not authenticated with GitHub CLI.\033[0m")
		fmt.Println("  Run: gh auth login")
		os.Exit(1)
	}

	// Check .env exists
	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		fmt.Println("  \033[31m✗ .env file not found.\033[0m")
		fmt.Println("  Create one from .env.example:")
		fmt.Println("    cp .env.example .env")
		os.Exit(1)
	}

	// Parse .env
	secrets := parseEnvFile(".env")
	if len(secrets) == 0 {
		fmt.Println("  \033[33m⚠ No secrets found in .env\033[0m")
		os.Exit(1)
	}

	fmt.Printf("  Found %d variables in .env\n", len(secrets))
	fmt.Println()

	// Confirm
	fmt.Print("  This will push ALL values to GitHub Secrets.")
	fmt.Println()
	fmt.Print("  Continue? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("  Cancelled.")
		return
	}

	fmt.Println()

	// Sync each secret
	synced := 0
	skipped := 0
	errors := 0

	for key, value := range secrets {
		if skipSecrets[key] {
			skipped++
			continue
		}

		// Skip keys that look like system vars
		if strings.HasPrefix(key, "_") || strings.HasPrefix(key, "BASH_") || strings.HasPrefix(key, "SHLVL") {
			skipped++
			continue
		}

		cmd := exec.Command("gh", "secret", "set", key, "--body", value)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("  \033[31m✗ %s\033[0m — %s\n", key, strings.TrimSpace(stderr.String()))
			errors++
		} else {
			fmt.Printf("  \033[32m✓ %s\033[0m\n", key)
			synced++
		}
	}

	fmt.Println()
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Printf("  Synced: %d  |  Skipped: %d  |  Errors: %d\n", synced, skipped, errors)
	if errors > 0 {
		fmt.Println("  \033[33mSome secrets failed — check error messages above.\033[0m")
		os.Exit(1)
	}
	fmt.Println("  \033[32mAll secrets synced successfully!\033[0m")
	fmt.Println()
}

func parseEnvFile(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Remove surrounding quotes
		value = strings.Trim(value, "\"'")
		// Skip empty values or references
		if value == "" || strings.HasPrefix(value, "${") {
			continue
		}
		result[key] = value
	}
	return result
}
