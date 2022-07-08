package step

import (
	"reflect"
	"testing"

	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/pathutil"
)

func Test_ProcessConfig(t *testing.T) {
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
				paths:   "/dev/null",
			},
			want: &Config{
				Verbose: false,
				Key:     "cache-key",
				Paths:   []string{"/dev/null"},
			},
			wantErr: false,
		},
		{
			name: "Multiple file paths",
			inputParser: fakeInputParser{
				verbose: false,
				key:     "cache-key",
				paths:   "/dev/null\n$BITRISE_SOURCE_DIR/node_modules\n~/.gradle",
			},
			want: &Config{
				Verbose: false,
				Key:     "cache-key",
				Paths:   []string{"/dev/null", "$BITRISE_SOURCE_DIR/node_modules", "~/.gradle"},
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
