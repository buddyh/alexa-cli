package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/buddyh/alexa-cli/internal/api"
	"github.com/buddyh/alexa-cli/internal/config"
	"github.com/spf13/cobra"
)

const cookieCLIVersion = "v5.0.1"

func newAuthCmd(flags *rootFlags) *cobra.Command {
	var domain string
	var country string

	cmd := &cobra.Command{
		Use:   "auth [refresh-token]",
		Short: "Configure authentication",
		Long: `Configure the Alexa CLI with your refresh token.

If no token is provided, opens a browser window for you to log into
Amazon and automatically captures the refresh token.

You can also provide a token directly:
  alexacli auth Atnr|...

Or set the ALEXA_REFRESH_TOKEN environment variable.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)
			country = normalizeLocalDomain(domain, country)

			var token string
			if len(args) > 0 {
				token = args[0]
			} else {
				// Try browser-based auth flow
				t, err := runBrowserAuth(domain, country)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Browser auth failed: %v\n", err)
					fmt.Fprintf(os.Stderr, "Falling back to manual token entry.\n\n")

					fmt.Print("Enter refresh token: ")
					reader := bufio.NewReader(os.Stdin)
					input, err := reader.ReadString('\n')
					if err != nil {
						return fmt.Errorf("failed to read input: %w", err)
					}
					token = strings.TrimSpace(input)
				} else {
					token = t
				}
			}

			if token == "" {
				return fmt.Errorf("refresh token is required")
			}

			// Validate the token by trying to authenticate
			client, err := api.NewClientWithLocal(token, domain, country)
			if err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}

			// Try to get devices to verify it works
			devices, err := client.GetDevices()
			if err != nil {
				return fmt.Errorf("failed to verify token: %w", err)
			}

			// Save configuration
			cfg := &config.Config{
				RefreshToken: token,
				AmazonDomain: domain,
				AmazonLocal:  country,
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			path, _ := config.Path()
			return out.Success(fmt.Sprintf("Authenticated successfully. Found %d devices. Config saved to %s", len(devices), path))
		},
	}

	cmd.Flags().StringVar(&domain, "domain", "amazon.com", "Base Amazon domain for login/token exchange (usually amazon.com)")
	cmd.Flags().StringVar(&country, "country", "", "Marketplace country page for login (defaults to --domain)")

	cmd.AddCommand(newAuthStatusCmd(flags))
	cmd.AddCommand(newAuthLogoutCmd(flags))

	return cmd
}

func newAuthStatusCmd(flags *rootFlags) *cobra.Command {
	var verify bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)

			cfg, err := config.Load()
			if err != nil {
				return out.Success("Not configured. Run 'alexacli auth' to set up.")
			}

			// Mask token: show first 8 chars + ...
			masked := cfg.RefreshToken
			if len(masked) > 8 {
				masked = masked[:8] + "..."
			}

			status := fmt.Sprintf("Domain: %s\nLocal:  %s\nToken:  %s", cfg.AmazonDomain, cfg.AmazonLocal, masked)

			if verify {
				client, err := api.NewClientWithLocal(cfg.RefreshToken, cfg.AmazonDomain, cfg.AmazonLocal)
				if err != nil {
					status += "\nStatus: invalid (authentication failed)"
					return out.Success(status)
				}
				devices, err := client.GetDevices()
				if err != nil {
					status += "\nStatus: invalid (failed to list devices)"
					return out.Success(status)
				}
				status += fmt.Sprintf("\nStatus: valid (%d devices)", len(devices))
			}

			path, _ := config.Path()
			status += fmt.Sprintf("\nConfig: %s", path)

			return out.Success(status)
		},
	}

	cmd.Flags().BoolVar(&verify, "verify", false, "Verify token by calling the API")

	return cmd
}

func newAuthLogoutCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := getFormatter(flags)

			path, err := config.Path()
			if err != nil {
				return err
			}

			if err := os.Remove(path); err != nil {
				if os.IsNotExist(err) {
					return out.Success("No credentials found.")
				}
				return fmt.Errorf("failed to remove config: %w", err)
			}

			return out.Success(fmt.Sprintf("Credentials removed from %s", path))
		},
	}
}

func normalizeLocalDomain(domain, country string) string {
	if country != "" {
		return country
	}
	if domain != "" {
		return domain
	}
	return "amazon.com"
}

// ensureCookieCLI checks for the alexa-cookie-cli binary and downloads it if missing.
// Returns the path to the binary.
func ensureCookieCLI() (string, error) {
	binDir, err := config.BinDir()
	if err != nil {
		return "", err
	}

	binPath := filepath.Join(binDir, "alexa-cookie-cli")

	// Check if already exists
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}

	// Determine platform name for the release URL
	var platform string
	switch runtime.GOOS {
	case "darwin":
		platform = "macos"
	case "linux":
		platform = "linux"
	case "windows":
		platform = "win"
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	url := fmt.Sprintf(
		"https://github.com/adn77/alexa-cookie-cli/releases/download/%s/alexa-cookie-cli-%s-x64",
		cookieCLIVersion, platform,
	)

	fmt.Fprintf(os.Stderr, "Downloading alexa-cookie-cli %s...\n", cookieCLIVersion)

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create bin directory: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(binPath)
	if err != nil {
		return "", fmt.Errorf("failed to create binary file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		os.Remove(binPath)
		return "", fmt.Errorf("download interrupted: %w", err)
	}

	if err := os.Chmod(binPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make binary executable: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Downloaded to %s\n", binPath)
	return binPath, nil
}

// localeFlags returns the locale arguments for alexa-cookie-cli based on the Amazon domain.
func localeFlags(domain string) []string {
	switch domain {
	case "amazon.de":
		return []string{"-a", "de_DE", "-L", "de-DE"}
	case "amazon.co.uk":
		return []string{"-a", "en_GB", "-L", "en-GB"}
	case "amazon.co.jp":
		return []string{"-a", "ja_JP", "-L", "ja-JP"}
	case "amazon.fr":
		return []string{"-a", "fr_FR", "-L", "fr-FR"}
	case "amazon.it":
		return []string{"-a", "it_IT", "-L", "it-IT"}
	case "amazon.es":
		return []string{"-a", "es_ES", "-L", "es-ES"}
	case "amazon.com.au":
		return []string{"-a", "en_AU", "-L", "en-AU"}
	case "amazon.ca":
		return []string{"-a", "en_CA", "-L", "en-CA"}
	case "amazon.com.br":
		return []string{"-a", "pt_BR", "-L", "pt-BR"}
	case "amazon.in":
		return []string{"-a", "en_IN", "-L", "en-IN"}
	default:
		return []string{"-a", "en_US", "-L", "en-US"}
	}
}

// runBrowserAuth launches alexa-cookie-cli to perform browser-based Amazon login.
// Returns the captured refresh token.
func runBrowserAuth(domain, country string) (string, error) {
	if domain == "" {
		domain = "amazon.com"
	}
	if country == "" {
		country = domain
	}

	binPath, err := ensureCookieCLI()
	if err != nil {
		return "", err
	}

	args := []string{"-b", domain, "-p", country}
	args = append(args, localeFlags(country)...)
	args = append(args, "-q")

	fmt.Fprintf(os.Stderr, "Opening browser for Amazon login at http://127.0.0.1:8080 ...\n")
	fmt.Fprintf(os.Stderr, "Log in to your Amazon account, then close the browser window.\n\n")

	cmd := exec.Command(binPath, args...)
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to capture output: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start alexa-cookie-cli: %w", err)
	}

	output, err := io.ReadAll(stdout)
	if err != nil {
		return "", fmt.Errorf("failed to read output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("alexa-cookie-cli exited with error: %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("no token received from browser login")
	}

	if !strings.HasPrefix(token, "Atnr|") {
		return "", fmt.Errorf("unexpected output (expected refresh token): %s", token)
	}

	return token, nil
}
