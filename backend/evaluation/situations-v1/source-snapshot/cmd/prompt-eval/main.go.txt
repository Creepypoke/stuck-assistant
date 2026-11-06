// Command prompt-eval compares local prompt profiles on real or oracle evidence.
package main

import (
	"fmt"
	"github.com/walkthrough-assistance/backend/evaluation/runner"
	"os"
)

func main() {
	if err := runner.Run("local"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
