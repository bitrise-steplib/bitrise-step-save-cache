package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-steplib/bitrise-step-save-cache/compression"
	"github.com/bitrise-steplib/bitrise-step-save-cache/network"

	"github.com/bitrise-io/go-steputils/v2/cache/keytemplate"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/docker/go-units"
)

type Input struct {
	Verbose bool   `env:"verbose,required"`
	Key     string `env:"key,required"`
	Paths   string `env:"paths,required"`
}

type Config struct {
	Verbose        bool
	Key            string
	Paths          []string
	APIBaseURL     stepconf.Secret
	APIAccessToken stepconf.Secret
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

func (step SaveCacheStep) ProcessConfig() (*Config, error) {
	var input Input
	if err := step.inputParser.Parse(&input); err != nil {
		return nil, err
	}
	stepconf.Print(input)
	step.logger.Println()

	if strings.TrimSpace(input.Key) == "" {
		return nil, fmt.Errorf("cache key should not be empty")
	}

	finalPaths, err := step.evaluatePaths(input.Paths)
	if err != nil {
		return nil, fmt.Errorf("failed to parse paths: %w", err)
	}

	apiBaseURL := step.envRepo.Get("BITRISEIO_ABCS_API_URL")
	if apiBaseURL == "" {
		return nil, fmt.Errorf("the secret 'BITRISEIO_ABCS_API_URL' is not defined")
	}
	apiAccessToken := step.envRepo.Get("BITRISEIO_ABCS_ACCESS_TOKEN")
	if apiAccessToken == "" {
		return nil, fmt.Errorf("the secret 'BITRISEIO_ABCS_ACCESS_TOKEN' is not defined")
	}

	return &Config{
		Verbose:        input.Verbose,
		Key:            input.Key,
		Paths:          finalPaths,
		APIBaseURL:     stepconf.Secret(apiBaseURL),
		APIAccessToken: stepconf.Secret(apiAccessToken),
	}, nil
}

func (step SaveCacheStep) Run(config Config) error {
	step.logger.Println()
	step.logger.Printf("Evaluating key template: %s", config.Key)
	evaluatedKey, err := step.evaluateKey(config.Key)
	if err != nil {
		return fmt.Errorf("failed to evaluate key template: %s", err)
	}
	step.logger.Donef("Cache key: %s", evaluatedKey)

	step.logger.Println()
	step.logger.Infof("Creating archive...")
	compressionStartTIme := time.Now()
	archivePath, err := step.compress(config.Paths)
	if err != nil {
		return fmt.Errorf("compression failed: %s", err)
	}
	step.logger.Donef("Archive created in %s", time.Since(compressionStartTIme).Round(time.Second))
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return err
	}
	step.logger.Printf("Archive size: %s", units.HumanSizeWithPrecision(float64(fileInfo.Size()), 3))
	step.logger.Debugf("Archive path: %s", archivePath)

	step.logger.Println()
	step.logger.Infof("Uploading archive...")
	uploadStartTime := time.Now()
	err = step.upload(archivePath, fileInfo.Size(), evaluatedKey, config)
	if err != nil {
		return fmt.Errorf("cache upload failed: %w", err)
	}
	step.logger.Donef("Archive uploaded in %s", time.Since(uploadStartTime).Round(time.Second))

	return nil
}

func (step SaveCacheStep) evaluatePaths(pathInput string) ([]string, error) {
	pathSlice := strings.Split(pathInput, "\n")

	// Expand wildcard paths
	var expandedPaths []string
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for _, path := range pathSlice {
		if !strings.Contains(path, "*") {
			expandedPaths = append(expandedPaths, path)
			continue
		}

		matches, err := doublestar.Glob(os.DirFS(workingDir), path)
		if matches == nil {
			step.logger.Warnf("No match for path pattern: %s", path)
			continue
		}
		if err != nil {
			step.logger.Warnf("Error in path pattern '%s': %w", path, err)
			continue
		}
		expandedPaths = append(expandedPaths, matches...)
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
	model := keytemplate.NewModel(step.envRepo, step.logger)
	buildContext := keytemplate.BuildContext{
		Workflow:   step.envRepo.Get("BITRISE_TRIGGERED_WORKFLOW_ID"),
		Branch:     step.envRepo.Get("BITRISE_GIT_BRANCH"),
		CommitHash: step.envRepo.Get("BITRISE_GIT_COMMIT"),
	}

	return model.Evaluate(keyTemplate, buildContext)
}

func (step SaveCacheStep) compress(paths []string) (string, error) {
	if compression.AreAllPathsEmpty(paths) {
		step.logger.Warnf("The provided paths are all empty, skipping compression and upload.")
		os.Exit(0)
	}

	fileName := fmt.Sprintf("cache-%s.tzst", time.Now().UTC().Format("20060102-150405"))
	tempDir, err := step.pathProvider.CreateTempDir("save-cache")
	if err != nil {
		return "", err
	}
	archivePath := filepath.Join(tempDir, fileName)

	err = compression.Compress(archivePath, paths, step.logger, step.envRepo)
	if err != nil {
		return "", err
	}

	return archivePath, nil
}

func (step SaveCacheStep) upload(archivePath string, archiveSize int64, cacheKey string, config Config) error {
	params := network.UploadParams{
		APIBaseURL:  string(config.APIBaseURL),
		Token:       string(config.APIAccessToken),
		ArchivePath: archivePath,
		ArchiveSize: archiveSize,
		CacheKey:    cacheKey,
	}
	return network.Upload(params, step.logger)
}
