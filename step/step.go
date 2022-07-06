package step

import (
	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
)

type Input struct {
	Verbose bool `env:"verbose,required"`
}

type Config struct {
	Verbose bool
}

type SaveCacheStep struct {
	logger         log.Logger
	inputParser    stepconf.InputParser
	commandFactory command.Factory
}

func New(logger log.Logger, inputParser stepconf.InputParser, commandFactory command.Factory) SaveCacheStep {
	return SaveCacheStep{
		logger:         logger,
		inputParser:    inputParser,
		commandFactory: commandFactory,
	}
}

func (step SaveCacheStep) ProcessConfig() (*Config, error) {
	var input Input
	if err := step.inputParser.Parse(&input); err != nil {
		return nil, err
	}

	return &Config{
		Verbose: input.Verbose,
	}, nil
}

func (step SaveCacheStep) Run(config Config) error {

	return nil
}
