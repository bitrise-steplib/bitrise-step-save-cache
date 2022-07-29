//go:build integration
// +build integration

package integration

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-steplib/steps-save-cache/compression"
)

func Test_compression(t *testing.T) {
	// Given
	archivePath := filepath.Join(t.TempDir(), "compression_test.tzst")
	logger := log.NewLogger()
	envRepo := fakeEnvRepo{envVars: map[string]string{
		"BITRISE_SOURCE_DIR": ".",
	}}

	// When
	err := compression.Compress(archivePath, []string{"../step/testdata/"}, logger, envRepo)
	if err != nil {
		t.Errorf(err.Error())
	}
	archiveContents, err := listArchiveContents(archivePath)
	if err != nil {
		t.Errorf(err.Error())
	}

	expected := []string{
		"../step/testdata/",
		"../step/testdata/subfolder/",
		"../step/testdata/dummy_file.txt",
		"../step/testdata/subfolder/nested_file.txt",
	}
	assert.Equal(t, expected, archiveContents)
}

func listArchiveContents(path string) ([]string, error) {
	output, err := command.NewFactory(env.NewRepository()).
		Create("tar", []string{"-tf", path}, nil).
		RunAndReturnTrimmedOutput()

	if err != nil {
		return nil, err
	}

	return strings.Split(output, "\n"), nil
}

type fakeEnvRepo struct {
	envVars map[string]string
}

func (repo fakeEnvRepo) Get(key string) string {
	value, ok := repo.envVars[key]
	if ok {
		return value
	} else {
		return ""
	}
}

func (repo fakeEnvRepo) Set(key, value string) error {
	repo.envVars[key] = value
	return nil
}

func (repo fakeEnvRepo) Unset(key string) error {
	repo.envVars[key] = ""
	return nil
}

func (repo fakeEnvRepo) List() []string {
	var values []string
	for _, v := range repo.envVars {
		values = append(values, v)
	}
	return values
}
