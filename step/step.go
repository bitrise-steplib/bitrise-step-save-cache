package step

import (
	"fmt"
	"strings"

	"github.com/bitrise-io/go-steputils/v2/cache/keytemplate"
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
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
	envRepo        env.Repository
}

func New(logger log.Logger, inputParser stepconf.InputParser, commandFactory command.Factory, pathChecker pathutil.PathChecker, envRepo env.Repository) SaveCacheStep {
	return SaveCacheStep{
		logger:         logger,
		inputParser:    inputParser,
		commandFactory: commandFactory,
		pathChecker:    pathChecker,
		envRepo:        envRepo,
	}
}

func (step SaveCacheStep) ProcessConfig() (*Config, error) {
	var input Input
	if err := step.inputParser.Parse(&input); err != nil {
		return nil, err
	}

	if strings.TrimSpace(input.Key) == "" {
		return nil, fmt.Errorf("cache key should not be empty")
	}

	pathSlice := strings.Split(input.Paths, "\n")
	for _, path := range pathSlice {
		if exists, _ := step.pathChecker.IsPathExists(path); !exists {
			step.logger.Warnf("Cache path doesn't exist: %s", path)
		}
	}

	return &Config{
		Verbose: input.Verbose,
		Key:     input.Key,
		Paths:   pathSlice,
	}, nil
}

func (step SaveCacheStep) Run(config Config) error {
	evaluatedKey, err := step.evaluateKey(config.Key)
	if err != nil {
		return fmt.Errorf("failed to evaluate key template: %s", err)
	}
	step.logger.Donef("Cache key: %s", evaluatedKey)

	return nil
}

func (step SaveCacheStep) evaluateKey(keyTemplate string) (string, error) {
	model := keytemplate.NewModel(step.envRepo, step.logger)
	buildContext := keytemplate.BuildContext{
		Workflow:   step.envRepo.Get("BITRISE_WORKFLOW_ID"),
		Branch:     step.envRepo.Get("BITRISE_GIT_BRANCH"),
		CommitHash: step.envRepo.Get("BITRISE_GIT_COMMIT"),
	}

	return model.Evaluate(keyTemplate, buildContext)
}
