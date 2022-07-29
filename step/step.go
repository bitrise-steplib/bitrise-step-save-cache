package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-steplib/steps-save-cache/compression"

	"github.com/bitrise-io/go-steputils/v2/cache/keytemplate"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/docker/go-units"
)

type Input struct {
	Verbose bool   `env:"verbose,required"`
	Key     string `env:"key,required"`
	Paths   string `env:"paths,required"`
}

type Config struct {
	Verbose bool
	Key     string
	Paths   []string
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

	var finalPaths []string
	pathSlice := strings.Split(input.Paths, "\n")
	for _, path := range pathSlice {
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

	return &Config{
		Verbose: input.Verbose,
		Key:     input.Key,
		Paths:   finalPaths,
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
	step.logger.Infof("Creating cache archive...")
	startTime := time.Now()
	archivePath, err := step.compress(config.Paths)
	if err != nil {
		return fmt.Errorf("compression failed: %s", err)
	}
	step.logger.Donef("Cache archive created in %s", time.Since(startTime).Round(time.Second))
	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		return err
	}
	step.logger.Printf("Archive size: %s", units.HumanSizeWithPrecision(float64(fileInfo.Size()), 3))
	step.logger.Debugf("Archive path: %s", archivePath)

	return nil
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
