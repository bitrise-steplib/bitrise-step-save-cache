package compression

import (
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
)

func Compress(archivePath string, includePaths []string, logger log.Logger, envRepo env.Repository) error {
	cmdFactory := command.NewFactory(envRepo)

	tarArgs := []string{
		"--use-compress-program",
		"zstd --threads=0 --long", // Use CPU count threads, enable long distance matching
		"-P",                      // Same as --absolute-paths in BSD tar, --absolute-names in GNU tar
		"-cf",
		archivePath,
		"--directory",
		envRepo.Get("BITRISE_SOURCE_DIR"),
	}
	tarArgs = append(tarArgs, includePaths...)

	cmd := cmdFactory.Create("tar", tarArgs, nil)

	logger.Debugf("$ %s", cmd.PrintableCommandArgs())

	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		logger.Errorf("Compression command failed: %s", out)
		return err
	}

	return nil
}
