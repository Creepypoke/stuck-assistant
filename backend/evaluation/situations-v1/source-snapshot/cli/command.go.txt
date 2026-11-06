package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/walkthrough-assistance/backend/ingestion"
	ingestionservice "github.com/walkthrough-assistance/backend/ingestion/service"
)

const (
	defaultEmbeddingURL        = "http://127.0.0.1:8081"
	defaultEmbeddingModel      = "embedding"
	defaultEmbeddingVersion    = "1"
	defaultEmbeddingDimensions = 768
	defaultChunkTokens         = 300
	defaultOverlapTokens       = 40
)

type Ingestor interface {
	Ingest(context.Context, ingestionservice.Request) (ingestion.Result, error)
}

type IngestorFactory func(Options) (Ingestor, error)

type Options struct {
	ParserCommand       string
	ParserArgs          []string
	StorageDirectory    string
	EmbeddingURL        string
	EmbeddingModel      string
	EmbeddingVersion    string
	EmbeddingDimensions int
	ChunkTokens         int
	OverlapTokens       int
	DownloadTimeout     time.Duration
	EmbeddingTimeout    time.Duration
	GameName            string
}

func NewCommand() *cobra.Command {
	return NewCommandWithFactory(NewIngestor)
}

func NewCommandWithFactory(factory IngestorFactory) *cobra.Command {
	logLevel := environmentOrDefault("WALKTHROUGH_LOG_LEVEL", "info")
	root := &cobra.Command{
		Use:           "walkthrough-assistance",
		Short:         "Local walkthrough assistance backend",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVar(&logLevel, "log-level", logLevel, "logging level: debug, info, warn, or error")
	root.AddCommand(newKnowledgeCommand(factory))
	root.AddCommand(newServiceCommand())
	root.AddCommand(newAskCommand())
	return root
}

func newKnowledgeCommand(factory IngestorFactory) *cobra.Command {
	knowledge := &cobra.Command{
		Use:   "knowledge",
		Short: "Manage locally stored walkthrough knowledge",
	}
	knowledge.AddCommand(newKnowledgeAddCommand(factory))
	return knowledge
}

func newKnowledgeAddCommand(factory IngestorFactory) *cobra.Command {
	options, configErr := optionsFromEnvironment()

	command := &cobra.Command{
		Use:   "add <game-id> <url-or-file>",
		Short: "Process and store a walkthrough from a URL or local file",
		Args:  cobra.ExactArgs(2),
		PreRunE: func(_ *cobra.Command, _ []string) error {
			return configErr
		},
		RunE: func(command *cobra.Command, args []string) error {
			ingestor, err := factory(options)
			if err != nil {
				return fmt.Errorf("configure ingestion: %w", err)
			}
			gameName := options.GameName
			if gameName == "" {
				gameName = args[0]
			}
			result, err := ingestor.Ingest(command.Context(), ingestionservice.Request{
				GameID: args[0], GameName: gameName, URL: args[1],
			})
			if err != nil {
				return err
			}
			printResult(command.OutOrStdout(), result)
			return nil
		},
	}

	flags := command.Flags()
	flags.StringVar(&options.GameName, "game-name", options.GameName, "human-readable game name (defaults to game-id)")
	flags.StringVar(&options.ParserCommand, "parser", options.ParserCommand, "parser executable")
	flags.StringArrayVar(&options.ParserArgs, "parser-arg", options.ParserArgs, "parser argument; may be specified multiple times")
	flags.StringVar(&options.StorageDirectory, "storage-dir", options.StorageDirectory, "directory for per-game .rag files")
	flags.StringVar(&options.EmbeddingURL, "embedding-url", options.EmbeddingURL, "llama.cpp embedding server base URL")
	flags.StringVar(&options.EmbeddingModel, "embedding-model", options.EmbeddingModel, "embedding model name")
	flags.StringVar(&options.EmbeddingVersion, "embedding-version", options.EmbeddingVersion, "embedding model version")
	flags.IntVar(&options.EmbeddingDimensions, "embedding-dimensions", options.EmbeddingDimensions, "embedding vector dimensions")
	flags.IntVar(&options.ChunkTokens, "chunk-tokens", options.ChunkTokens, "maximum estimated tokens per chunk")
	flags.IntVar(&options.OverlapTokens, "overlap-tokens", options.OverlapTokens, "estimated overlap tokens between chunks")
	flags.DurationVar(&options.DownloadTimeout, "download-timeout", options.DownloadTimeout, "HTTP walkthrough download timeout")
	flags.DurationVar(&options.EmbeddingTimeout, "embedding-timeout", options.EmbeddingTimeout, "embedding request timeout")
	return command
}

func optionsFromEnvironment() (Options, error) {
	options := Options{
		ParserCommand:       defaultParserCommand(),
		StorageDirectory:    defaultStorageDirectory(),
		EmbeddingURL:        defaultEmbeddingURL,
		EmbeddingModel:      defaultEmbeddingModel,
		EmbeddingVersion:    defaultEmbeddingVersion,
		EmbeddingDimensions: defaultEmbeddingDimensions,
		ChunkTokens:         defaultChunkTokens,
		OverlapTokens:       defaultOverlapTokens,
		DownloadTimeout:     30 * time.Second,
		EmbeddingTimeout:    2 * time.Minute,
	}

	options.GameName = os.Getenv("WALKTHROUGH_GAME_NAME")
	if parserCommand := os.Getenv("WALKTHROUGH_PARSER"); parserCommand != "" {
		options.ParserCommand = resolveExecutableRelativeCommand(parserCommand)
	}
	options.ParserArgs = resolveExecutableRelativePaths(strings.Fields(os.Getenv("WALKTHROUGH_PARSER_ARGS")))
	if storageDirectory := os.Getenv("WALKTHROUGH_STORAGE_DIR"); storageDirectory != "" {
		options.StorageDirectory = resolveExecutableRelativePath(storageDirectory)
	}
	options.EmbeddingURL = environmentOrDefault("WALKTHROUGH_EMBEDDING_URL", options.EmbeddingURL)
	options.EmbeddingModel = environmentOrDefault("WALKTHROUGH_EMBEDDING_MODEL", options.EmbeddingModel)
	options.EmbeddingVersion = environmentOrDefault("WALKTHROUGH_EMBEDDING_VERSION", options.EmbeddingVersion)

	var err error
	if options.EmbeddingDimensions, err = environmentInt("WALKTHROUGH_EMBEDDING_DIMENSIONS", options.EmbeddingDimensions); err != nil {
		return Options{}, err
	}
	if options.ChunkTokens, err = environmentInt("WALKTHROUGH_CHUNK_TOKENS", options.ChunkTokens); err != nil {
		return Options{}, err
	}
	if options.OverlapTokens, err = environmentInt("WALKTHROUGH_OVERLAP_TOKENS", options.OverlapTokens); err != nil {
		return Options{}, err
	}
	if options.DownloadTimeout, err = environmentDuration("WALKTHROUGH_DOWNLOAD_TIMEOUT", options.DownloadTimeout); err != nil {
		return Options{}, err
	}
	if options.EmbeddingTimeout, err = environmentDuration("WALKTHROUGH_EMBEDDING_TIMEOUT", options.EmbeddingTimeout); err != nil {
		return Options{}, err
	}
	return options, nil
}

func defaultParserCommand() string {
	executable, err := os.Executable()
	if err == nil {
		parser := filepath.Join(filepath.Dir(executable), "walkthrough-parser")
		if info, statErr := os.Stat(parser); statErr == nil && !info.IsDir() {
			return parser
		}
	}
	return "walkthrough-parser"
}

func resolveExecutableRelativePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	executable, err := os.Executable()
	if err != nil {
		return path
	}
	return filepath.Join(filepath.Dir(executable), path)
}

func resolveExecutableRelativePaths(paths []string) []string {
	for index, path := range paths {
		paths[index] = resolveExecutableRelativeCommand(path)
	}
	return paths
}

// resolveExecutableRelativeCommand keeps command names and option values intact.
// A relative file path must contain a directory component (for example, ./parser.py).
func resolveExecutableRelativeCommand(value string) string {
	if value == "" || strings.HasPrefix(value, "-") || !strings.ContainsRune(value, filepath.Separator) {
		return value
	}
	return resolveExecutableRelativePath(value)
}

func environmentOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func environmentInt(key string, defaultValue int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func environmentDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func defaultStorageDirectory() string {
	dataDirectory, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("data", "games")
	}
	return filepath.Join(dataDirectory, ".local", "share", "walkthrough-assistance", "games")
}

func printResult(output io.Writer, result ingestion.Result) {
	fmt.Fprintf(output, "document_id: %d\nchunks: %d\npath: %s\n", result.DocumentID, result.ChunkCount, result.RAGPath)
}
