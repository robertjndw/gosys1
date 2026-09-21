// Command config shows which of TYPESAFE_API_KEY, TYPESAFE_BASE_URL,
// TYPESAFE_DEFAULT_MODEL and TYPESAFE_LOG_LEVEL New picks up from the
// environment, and that explicit options override them. It requires
// TYPESAFE_API_KEY; the other three are optional.
package main

import (
	"fmt"
	"os"
	"strings"

	sys1 "github.com/robertjndw/gosys1"
)

func main() {
	for _, name := range []string{sys1.EnvAPIKey, sys1.EnvBaseURL, sys1.EnvModel, sys1.EnvLogLevel} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			fmt.Printf("%s picked up from environment\n", name)
		} else {
			fmt.Printf("%s not set (default applies)\n", name)
		}
	}

	// Precedence is explicit option, then environment variable, then
	// package default; New applies it, so nothing here needs to
	// re-derive the effective values.
	if _, err := sys1.New(); err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client from environment:", err)
		os.Exit(1)
	}
	fmt.Println("client built from environment and defaults")

	const explicitBaseURL = "https://staging.typesafe.ai"
	const explicitModel = "jev-1.13.0"
	if _, err := sys1.New(sys1.WithBaseURL(explicitBaseURL), sys1.WithModel(explicitModel)); err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client with explicit options:", err)
		os.Exit(1)
	}
	fmt.Printf("client built with explicit base URL %s and model %s, overriding %s and %s\n",
		explicitBaseURL, explicitModel, sys1.EnvBaseURL, sys1.EnvModel)
}
