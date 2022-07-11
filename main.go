package main

import (
	"os"

	"github.com/bitrise-io/go-steputils/v2/stepconf"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
	"github.com/bitrise-steplib/steps-save-cache/step"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger := log.NewLogger()
	envRepo := env.NewRepository()
	inputParser := stepconf.NewInputParser(envRepo)
	cmdFactory := command.NewFactory(envRepo)
	pathChecker := pathutil.NewPathChecker()
	cacheStep := step.New(logger, inputParser, cmdFactory, pathChecker)

	exitCode := 0

	config, err := cacheStep.ProcessConfig()
	if err != nil {
		logger.Errorf(err.Error())
		exitCode = 1
		return exitCode
	}

	logger.EnableDebugLog(config.Verbose)

	if err := cacheStep.Run(*config); err != nil {
		logger.Errorf(err.Error())
		exitCode = 1
		return exitCode
	}

	return exitCode
}
