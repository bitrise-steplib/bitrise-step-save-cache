package step

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-steplib/bitrise-step-save-cache/internal/uploader"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/docker/go-units"
)

type Input struct {
	Verbose          bool   `env:"verbose,required"`
	Key              string `env:"key,required"`
	Paths            string `env:"paths,required"`
	IsKeyUnique      bool   `env:"is_key_unique"`
	CustomTarArgs    string `env:"custom_tar_args"`
	CompressChunks   bool   `env:"compress_chunks"`
	CompressionLevel int    `env:"compression_level,range[1..19]"`
}

type SaveCacheStep struct {
	logger         log.Logger
	inputParser    stepconf.InputParser
	commandFactory command.Factory
	pathChecker    pathutil.PathChecker
	pathProvider   pathutil.PathProvider
	pathModifier   pathutil.PathModifier
	envRepo        env.Repository
}

func New(logger log.Logger, inputParser stepconf.InputParser, commandFactory command.Factory, pathChecker pathutil.PathChecker, pathProvider pathutil.PathProvider, pathModifier pathutil.PathModifier, envRepo env.Repository) SaveCacheStep {
	return SaveCacheStep{
		logger:         logger,
		inputParser:    inputParser,
		commandFactory: commandFactory,
		pathChecker:    pathChecker,
		pathProvider:   pathProvider,
		pathModifier:   pathModifier,
		envRepo:        envRepo,
	}
}

func (step SaveCacheStep) Run() error {
	var input Input
	if err := step.inputParser.Parse(&input); err != nil {
		return fmt.Errorf("failed to parse inputs: %w", err)
	}
	stepconf.Print(input)
	step.logger.Println()

	step.logger.EnableDebugLog(input.Verbose)

	// Parse configuration
	config, err := step.createConfig(input)
	if err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	step.logger.Println()
	step.logger.Printf("Evaluating key template: %s", input.Key)
	evaluatedKey, err := step.evaluateKey(input.Key)
	if err != nil {
		return fmt.Errorf("failed to evaluate key template: %w", err)
	}
	step.logger.Donef("Cache key: %s", evaluatedKey)

	step.logger.Println()
	step.logger.Infof("Evaluating paths...")
	finalPaths, err := step.evaluatePaths(strings.Split(input.Paths, "\n"))
	if err != nil {
		return fmt.Errorf("failed to evaluate paths: %w", err)
	}

	if len(finalPaths) == 0 {
		step.logger.Warnf("No valid paths to cache, skipping upload")
		return nil
	}

	step.logger.Donef("Found %d paths to cache", len(finalPaths))
	for _, path := range finalPaths {
		step.logger.Printf("  - %s", path)
	}

	// Create temporary UNCOMPRESSED archive for chunking
	// Note: We don't compress before chunking because CDC works poorly with compressed data
	step.logger.Println()
	step.logger.Infof("Creating uncompressed archive for chunking...")
	step.logger.Infof("(CDC works best on uncompressed data for better deduplication)")
	archiveStartTime := time.Now()
	archivePath, err := step.createUncompressedArchive(finalPaths, strings.Fields(input.CustomTarArgs))
	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}
	defer os.Remove(archivePath)
	archiveTime := time.Since(archiveStartTime).Round(time.Second)
	step.logger.Donef("Archive created in %s", archiveTime)

	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}
	step.logger.Printf("Uncompressed archive size: %s", units.HumanSizeWithPrecision(float64(fileInfo.Size()), 3))
	step.logger.Debugf("Archive path: %s", archivePath)

	// Upload using chunked uploader
	step.logger.Println()
	uploadStartTime := time.Now()
	chunkedUploader := uploader.NewChunkedUploader(config.APIBaseURL, config.APIAccessToken, step.logger)
	err = chunkedUploader.Upload(context.Background(), uploader.UploadParams{
		FilePath:         archivePath,
		CacheKey:         evaluatedKey,
		CompressChunks:   config.CompressChunks,
		CompressionLevel: config.CompressionLevel,
	})
	if err != nil {
		return fmt.Errorf("chunked upload failed: %w", err)
	}
	uploadTime := time.Since(uploadStartTime).Round(time.Second)

	step.logger.Println()
	step.logger.Donef("✅ Cache saved successfully in %s", uploadTime)

	return nil
}

type config struct {
	Verbose          bool
	Key              string
	Paths            []string
	CustomTarArgs    []string
	APIBaseURL       string
	APIAccessToken   string
	CompressChunks   bool
	CompressionLevel int
}

func (step SaveCacheStep) createConfig(input Input) (config, error) {
	if strings.TrimSpace(input.Key) == "" {
		return config{}, fmt.Errorf("cache key should not be empty")
	}

	apiBaseURL := step.envRepo.Get("BITRISEIO_ABCS_API_URL")
	if apiBaseURL == "" {
		return config{}, fmt.Errorf("the secret 'BITRISEIO_ABCS_API_URL' is not defined")
	}
	apiAccessToken := step.envRepo.Get("BITRISEIO_BITRISE_SERVICES_ACCESS_TOKEN")
	if apiAccessToken == "" {
		return config{}, fmt.Errorf("the secret 'BITRISEIO_BITRISE_SERVICES_ACCESS_TOKEN' is not defined")
	}

	compressionLevel := input.CompressionLevel
	if compressionLevel == 0 {
		compressionLevel = 3
	}
	if compressionLevel < 1 || compressionLevel > 19 {
		return config{}, fmt.Errorf("compression level should be between 1 and 19")
	}

	return config{
		Verbose:          input.Verbose,
		Key:              input.Key,
		Paths:            strings.Split(input.Paths, "\n"),
		CustomTarArgs:    strings.Fields(input.CustomTarArgs),
		APIBaseURL:       apiBaseURL,
		APIAccessToken:   apiAccessToken,
		CompressChunks:   input.CompressChunks,
		CompressionLevel: compressionLevel,
	}, nil
}

func (step SaveCacheStep) evaluatePaths(paths []string) ([]string, error) {
	// Expand wildcard paths
	var expandedPaths []string
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}

		if !strings.Contains(path, "*") {
			expandedPaths = append(expandedPaths, path)
			continue
		}

		base, pattern := doublestar.SplitPattern(path)
		absBase, err := step.pathModifier.AbsPath(base)
		if err != nil {
			return nil, err
		}
		matches, err := doublestar.Glob(os.DirFS(absBase), pattern, doublestar.WithNoFollow())
		if matches == nil {
			step.logger.Warnf("No match for path pattern: %s", path)
			continue
		}
		if err != nil {
			step.logger.Warnf("Error in path pattern '%s': %v", path, err)
			continue
		}

		for _, match := range matches {
			expandedPaths = append(expandedPaths, filepath.Join(base, match))
		}
	}

	// Validate and sanitize paths
	var finalPaths []string
	for _, path := range expandedPaths {
		absPath, err := step.pathModifier.AbsPath(path)
		if err != nil {
			step.logger.Warnf("Failed to parse path %s, error: %s", path, err)
			continue
		}

		exists, err := step.pathChecker.IsPathExists(absPath)
		if err != nil {
			step.logger.Warnf("Failed to check path %s, error: %s", absPath, err)
		}
		if !exists {
			step.logger.Warnf("Cache path doesn't exist: %s", path)
			continue
		}

		finalPaths = append(finalPaths, absPath)
	}

	return finalPaths, nil
}

func (step SaveCacheStep) evaluateKey(keyTemplate string) (string, error) {
	// For now, use simple template evaluation
	// This should be replaced with the actual keytemplate package implementation
	// from go-steputils if needed
	key := keyTemplate

	// Basic template variables
	key = strings.ReplaceAll(key, "{{ .OS }}", step.envRepo.Get("GOOS"))
	key = strings.ReplaceAll(key, "{{ .Arch }}", step.envRepo.Get("GOARCH"))
	key = strings.ReplaceAll(key, "{{ .Branch }}", step.envRepo.Get("BITRISE_GIT_BRANCH"))
	key = strings.ReplaceAll(key, "{{ .CommitHash }}", step.envRepo.Get("BITRISE_GIT_COMMIT"))
	key = strings.ReplaceAll(key, "{{ .Workflow }}", step.envRepo.Get("BITRISE_TRIGGERED_WORKFLOW_ID"))

	// Truncate if too long
	maxKeyLength := 512
	if len(key) > maxKeyLength {
		step.logger.Warnf("Key is too long, truncating to %d characters", maxKeyLength)
		key = key[:maxKeyLength]
	}

	// Validate no commas
	if strings.Contains(key, ",") {
		return "", fmt.Errorf("commas are not allowed in cache key")
	}

	return key, nil
}

func (step SaveCacheStep) createUncompressedArchive(paths []string, customTarArgs []string) (string, error) {
	fileName := fmt.Sprintf("cache-%s.tar", time.Now().UTC().Format("20060102-150405"))
	tempDir, err := step.pathProvider.CreateTempDir("save-cache")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	archivePath := filepath.Join(tempDir, fileName)

	// Build tar command WITHOUT compression (CDC works better on uncompressed data)
	tarArgs := []string{
		"--create",
		"--file", archivePath,
	}

	// Add custom tar args if provided
	if len(customTarArgs) > 0 {
		tarArgs = append(tarArgs, customTarArgs...)
	}

	// Add paths
	tarArgs = append(tarArgs, paths...)

	cmd := step.commandFactory.Create("tar", tarArgs, &command.Opts{})
	step.logger.Debugf("Running: tar %s", strings.Join(tarArgs, " "))

	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tar command failed: %w\nOutput: %s", err, out)
	}

	return archivePath, nil
}
