package dogeboxd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueueManagementSkipsAdjacentNixCacheUpdates(t *testing.T) {
	dbx := Dogeboxd{
		queue: &syncQueue{
			jobQueue: []Job{
				{ID: "cache-1", A: UpdateNixCache{}},
			},
		},
	}

	assert.True(t, dbx.shouldSkipJob(Job{ID: "cache-2", A: UpdateNixCache{}}))
}

func TestQueueManagementKeepsSeparatedNixCacheUpdates(t *testing.T) {
	dbx := Dogeboxd{
		queue: &syncQueue{
			jobQueue: []Job{
				{ID: "cache-1", A: UpdateNixCache{}},
				{ID: "job-2", A: UpdateTimezone{Timezone: "UTC"}},
			},
		},
	}

	assert.False(t, dbx.shouldSkipJob(Job{ID: "cache-3", A: UpdateNixCache{}}))
}

func TestQueueManagementIgnoresRunningNixCacheJob(t *testing.T) {
	dbx := Dogeboxd{
		queue: &syncQueue{},
	}

	assert.False(t, dbx.shouldSkipJob(Job{ID: "cache-1", A: UpdateNixCache{}}))
}
