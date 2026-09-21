// Command config shows which of TYPESAFE_API_KEY, TYPESAFE_BASE_URL,
// TYPESAFE_DEFAULT_MODEL and TYPESAFE_LOG_LEVEL New picks up from the
// environment, an explicit WithBaseURL option overriding it, and
// client.WithModel and client.WithTimeout deriving a copy that
// overrides the model and per-attempt timeout in code. It requires
// TYPESAFE_API_KEY; the other three are optional.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

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
	client, err := sys1.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client from environment:", err)
		os.Exit(1)
	}
	fmt.Println("client built from environment and defaults")

	const explicitBaseURL = "https://staging.typesafe.ai"
	if _, err := sys1.New(sys1.WithBaseURL(explicitBaseURL)); err != nil {
		fmt.Fprintln(os.Stderr, "sys1: creating client with explicit base URL:", err)
		os.Exit(1)
	}
	fmt.Printf("client built with explicit base URL %s, overriding %s\n", explicitBaseURL, sys1.EnvBaseURL)

	// TYPESAFE_DEFAULT_MODEL sets the model at New time; to change it,
	// or the per-attempt timeout, in code instead, derive a copy.
	// client itself is unchanged.
	const explicitModel = "jev-1.13.0"
	const explicitTimeout = 3 * time.Second
	tuned := client.WithModel(explicitModel).WithTimeout(explicitTimeout)
	_ = tuned // ready for calls; this example makes none
	fmt.Printf("derived client uses model %s with a %s per-attempt timeout, overriding %s\n",
		explicitModel, explicitTimeout, sys1.EnvModel)
}
