package config

import (
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/stretchr/testify/assert"
)

func TestRedisStaleConnsTracker_CumulativeCounterDoesNotAccumulate(t *testing.T) {
	client := &redisclient.Client{}
	var tracker redisStaleConnsTracker

	// Production samples from OPS-74: cumulative counter far above a pool of ~51 connections.
	first := tracker.observe(client, 13395, 51)
	assert.Equal(t, uint32(13395), first.RemovedTotal)
	assert.Equal(t, uint32(0), first.RemovedInInterval, "first sample only sets the baseline")
	assert.False(t, first.Alert)

	steady := tracker.observe(client, 13395, 51)
	assert.Equal(t, uint32(0), steady.RemovedInInterval)
	assert.Zero(t, steady.Percent)
	assert.False(t, steady.Alert, "a large but unchanged cumulative counter must not alert")

	normalChurn := tracker.observe(client, 13397, 51)
	assert.Equal(t, uint32(2), normalChurn.RemovedInInterval)
	assert.InDelta(t, 2.0/51.0*100, normalChurn.Percent, 0.0001)
	assert.False(t, normalChurn.Alert, "routine idle/lifetime rotation stays below the threshold")
}

func TestRedisStaleConnsTracker_AlertsOnBurstWithinInterval(t *testing.T) {
	client := &redisclient.Client{}
	var tracker redisStaleConnsTracker

	tracker.observe(client, 100, 50)
	atThreshold := tracker.observe(client, 110, 50)
	assert.Equal(t, uint32(10), atThreshold.RemovedInInterval)
	assert.InDelta(t, 20.0, atThreshold.Percent, 0.0001)
	assert.False(t, atThreshold.Alert, "exactly 20% is not above the threshold")

	burst := tracker.observe(client, 121, 50)
	assert.Equal(t, uint32(11), burst.RemovedInInterval)
	assert.InDelta(t, 22.0, burst.Percent, 0.0001)
	assert.True(t, burst.Alert)

	afterBurst := tracker.observe(client, 121, 50)
	assert.False(t, afterBurst.Alert, "alert clears once removals stop")
}

func TestRedisStaleConnsTracker_NewClientResetsBaseline(t *testing.T) {
	oldClient := &redisclient.Client{}
	newClient := &redisclient.Client{}
	var tracker redisStaleConnsTracker

	tracker.observe(oldClient, 5000, 50)
	swapped := tracker.observe(newClient, 40, 50)
	assert.Equal(t, uint32(0), swapped.RemovedInInterval, "a replaced client starts a new baseline")
	assert.False(t, swapped.Alert)

	next := tracker.observe(newClient, 42, 50)
	assert.Equal(t, uint32(2), next.RemovedInInterval)
}

func TestRedisStaleConnsTracker_CounterWentBackwards(t *testing.T) {
	client := &redisclient.Client{}
	var tracker redisStaleConnsTracker

	tracker.observe(client, 5000, 50)
	reset := tracker.observe(client, 3, 50)
	assert.Equal(t, uint32(0), reset.RemovedInInterval, "a counter reset starts a new baseline, never a wrapped uint32 delta")
	assert.False(t, reset.Alert)

	next := tracker.observe(client, 5, 50)
	assert.Equal(t, uint32(2), next.RemovedInInterval)
}

func TestRedisStaleConnsTracker_ClusterNodeLeavingDoesNotAlert(t *testing.T) {
	client := &redisclient.Client{}
	var tracker redisStaleConnsTracker

	// ClusterClient.PoolStats sums every node; a node leaving lowers the summed counter.
	tracker.observe(client, 5000, 50)
	nodeLeft := tracker.observe(client, 4990, 50)
	assert.Equal(t, uint32(0), nodeLeft.RemovedInInterval)
	assert.False(t, nodeLeft.Alert, "a smaller cluster-wide sum is not a burst of stale removals")

	next := tracker.observe(client, 4993, 50)
	assert.Equal(t, uint32(3), next.RemovedInInterval)
	assert.False(t, next.Alert)
}

func TestRedisStaleConnsTracker_EmptyPoolHasNoPercent(t *testing.T) {
	client := &redisclient.Client{}
	var tracker redisStaleConnsTracker

	tracker.observe(client, 0, 0)
	empty := tracker.observe(client, 7, 0)
	assert.Equal(t, uint32(7), empty.RemovedInInterval)
	assert.Zero(t, empty.Percent, "no division by an empty pool (no NaN/Inf)")
	assert.False(t, empty.Alert)
}
