package step

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
)

func Test_ProcessConfig(t *testing.T) {
	testdataAbsPath, err := filepath.Abs("testdata")
	if err != nil {
		t.Errorf(err.Error())
	}

	tests := []struct {
		name        string
		inputParser fakeInputParser
		want        *Config
		wantErr     bool
	}{
		{
			name: "Invalid key input",
			inputParser: fakeInputParser{
				verbose: false,
				key:     "  ",
				paths:   "/dev/null",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "Single file path",
			inputParser: fakeInputParser{
				verbose: false,
				key:     "cache-key",
				paths:   "testdata/dummy_file.txt",
			},
			want: &Config{
				Verbose: false,
				Key:     "cache-key",
				Paths:   []string{filepath.Join(testdataAbsPath, "dummy_file.txt")},
			},
			wantErr: false,
		},
		{
			name: "Multiple file paths",
			inputParser: fakeInputParser{
				verbose: false,
				key:     "cache-key",
				paths:   "testdata/dummy_file.txt\ntestdata/subfolder/nested_file.txt",
			},
			want: &Config{
				Verbose: false,
				Key:     "cache-key",
				Paths:   []string{filepath.Join(testdataAbsPath, "dummy_file.txt"), filepath.Join(testdataAbsPath, "subfolder", "nested_file.txt")},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := SaveCacheStep{
				logger:         log.NewLogger(),
				inputParser:    tt.inputParser,
				commandFactory: command.NewFactory(env.NewRepository()),
				pathChecker:    pathutil.NewPathChecker(),
				pathProvider:   pathutil.NewPathProvider(),
				pathModifier:   pathutil.NewPathModifier(),
			}
			got, err := step.ProcessConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProcessConfig() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_evaluateKey(t *testing.T) {
	type args struct {
		keyTemplate string
		envRepo     fakeEnvRepo
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Happy path",
			args: args{
				keyTemplate: "npm-cache-{{ .Branch }}",
				envRepo: fakeEnvRepo{envVars: map[string]string{
					"BITRISE_TRIGGERED_WORKFLOW_ID": "primary",
					"BITRISE_GIT_BRANCH":            "main",
					"BITRISE_GIT_COMMIT":            "9de033412f24b70b59ca8392ccb9f61ac5af4cc3",
				}},
			},
			want:    "npm-cache-main",
			wantErr: false,
		},
		{
			name: "Empty env vars",
			args: args{
				keyTemplate: "npm-cache-{{ .Branch }}",
				envRepo:     fakeEnvRepo{},
			},
			want:    "npm-cache-",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := SaveCacheStep{
				logger:         log.NewLogger(),
				inputParser:    fakeInputParser{},
				commandFactory: command.NewFactory(env.NewRepository()),
				pathChecker:    pathutil.NewPathChecker(),
				envRepo:        tt.args.envRepo,
			}
			got, err := step.evaluateKey(tt.args.keyTemplate)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("evaluateKey() got = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeInputParser struct {
	verbose bool
	key     string
	paths   string
}

func (p fakeInputParser) Parse(input interface{}) error {
	inputRef := input.(*Input)
	inputRef.Verbose = p.verbose
	inputRef.Key = p.key
	inputRef.Paths = p.paths

	return nil
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
