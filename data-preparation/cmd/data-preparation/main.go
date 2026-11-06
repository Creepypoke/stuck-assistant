package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/walkthrough-assistance/data-preparation/pipeline"
)

func command() *cobra.Command {
	var config, walkthrough string
	root := &cobra.Command{Use: "data-preparation", Short: "Prepare local walkthroughs and build .rag databases", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&config, "config", "data-preparation.yaml", "Shared CLI YAML configuration with API provider profiles")
	root.PersistentFlags().StringVar(&walkthrough, "walkthrough", "", "Walkthrough YAML configuration (required; paths relative to this file)")
	if err := root.MarkPersistentFlagRequired("walkthrough"); err != nil {
		panic(err)
	}
	for _, action := range []string{"prepare", "validate", "build", "run"} {
		action := action
		short := map[string]string{"prepare": "Extract source blocks and prepare YAML cards", "validate": "Check artifacts, source evidence and review status", "build": "Build a .rag from prepared artifacts and embeddings", "run": "Prepare artifacts and build a .rag"}[action]
		c := &cobra.Command{Use: action, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, e := pipeline.LoadConfig(config, walkthrough)
			if e != nil {
				return e
			}
			ctx := cmd.Context()
			switch action {
			case "prepare":
				e = pipeline.Prepare(ctx, cfg)
			case "validate":
				e = pipeline.Validate(cfg)
			case "build":
				e = pipeline.Build(ctx, cfg)
			case "run":
				e = pipeline.Prepare(ctx, cfg)
				if e == nil {
					e = pipeline.Build(ctx, cfg)
				}
			}
			if e == nil {
				fmt.Fprintln(cmd.OutOrStdout(), action+": completed")
			}
			return e
		}}
		root.AddCommand(c)
	}
	return root
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if e := command().ExecuteContext(ctx); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
