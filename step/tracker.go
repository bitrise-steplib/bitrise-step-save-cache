package step

import (
	"io/fs"
	"time"

	"github.com/bitrise-io/go-utils/v2/analytics"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
)

type stepTracker struct {
	tracker analytics.Tracker
	logger  log.Logger
}

func newStepTracker(config Config, envRepo env.Repository, logger log.Logger) stepTracker {
	p := analytics.Properties{
		"build_slug":  envRepo.Get("BITRISE_BUILD_SLUG"),
		"app_slug":    envRepo.Get("BITRISE_APP_SLUG"),
		"workflow":    envRepo.Get("BITRISE_TRIGGERED_WORKFLOW_ID"),
		"is_pr_build": envRepo.Get("IS_PR") == "true",
		"path_count":  len(config.Paths),
	}
	return stepTracker{
		tracker: analytics.NewDefaultTracker(logger, p),
		logger:  logger,
	}
}

func (t *stepTracker) logArchiveUploaded(uploadTime time.Duration, info fs.FileInfo) {
	properties := analytics.Properties{
		"upload_time_s":     uploadTime.Truncate(time.Second).Seconds(),
		"upload_size_bytes": info.Size(),
	}
	t.tracker.Enqueue("step_save_cache_archive_uploaded", properties)
}

func (t *stepTracker) logArchiveCompressed(compressionTime time.Duration) {
	properties := analytics.Properties{
		"compression_time_s": compressionTime.Truncate(time.Second).Seconds(),
	}
	t.tracker.Enqueue("step_save_cache_archive_compressed", properties)
}

func (t *stepTracker) wait() {
	t.tracker.Wait()
}