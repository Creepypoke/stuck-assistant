// Command retrieval-eval runs retrieval diagnostics without requiring generation.
package main

import (
	"fmt"
	"github.com/walkthrough-assistance/backend/evaluation/runner"
	"os"
)

func main() {
	if err := runner.Run("retrieval"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
