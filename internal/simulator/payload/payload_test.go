package payload

import (
	"encoding/json"
	"testing"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/watch"
)

func TestBuildApplication_RespectsResourceCount(t *testing.T) {
	for _, n := range []int{0, 1, 5, 20, 100, 500} {
		app := BuildApplication(42, n, nil)
		assert.Len(t, app.Status.Resources, n, "count=%d", n)
	}
}

func TestBuildApplication_DeterministicPerSeed(t *testing.T) {
	a := BuildApplication(1234, 20, nil)
	b := BuildApplication(1234, 20, nil)
	aj, err := json.Marshal(a)
	require.NoError(t, err)
	bj, err := json.Marshal(b)
	require.NoError(t, err)
	assert.Equal(t, string(aj), string(bj), "same seed+count must produce byte-identical JSON")
}

func TestBuildApplication_DifferentSeedsDiffer(t *testing.T) {
	a := BuildApplication(1, 10, nil)
	b := BuildApplication(2, 10, nil)
	aj, err := json.Marshal(a)
	require.NoError(t, err)
	bj, err := json.Marshal(b)
	require.NoError(t, err)
	assert.NotEqual(t, string(aj), string(bj))
}

func TestBuildEventJSON_RoundTripsToApplicationWatchEvent(t *testing.T) {
	js, err := BuildEventJSON(7, 5, nil)
	require.NoError(t, err)

	var evt v1alpha1.ApplicationWatchEvent
	err = json.Unmarshal([]byte(js), &evt)
	require.NoError(t, err)
	assert.Equal(t, watch.Modified, evt.Type)
	assert.Len(t, evt.Application.Status.Resources, 5)
	assert.NotEmpty(t, evt.Application.ObjectMeta.Name)
	assert.NotEmpty(t, evt.Application.ObjectMeta.UID)
}

func TestSizeClass_ResourceCount(t *testing.T) {
	cases := map[SizeClass]int{
		SizeSmall:  5,
		SizeMedium: 20,
		SizeLarge:  100,
	}
	for c, expect := range cases {
		got, err := c.ResourceCount()
		require.NoError(t, err, "size=%s", c)
		assert.Equal(t, expect, got, "size=%s", c)
	}
	_, err := SizeClass("bogus").ResourceCount()
	assert.Error(t, err)
}

func TestCache_PrebuildsAndServesAllClasses(t *testing.T) {
	c, err := NewCache(99, nil, SizeSmall, SizeMedium, SizeLarge)
	require.NoError(t, err)

	for _, class := range []SizeClass{SizeSmall, SizeMedium, SizeLarge} {
		js, err := c.Get(class)
		require.NoError(t, err, "class=%s", class)
		assert.NotEmpty(t, js)
		var evt v1alpha1.ApplicationWatchEvent
		require.NoError(t, json.Unmarshal([]byte(js), &evt), "class=%s", class)
		count, _ := class.ResourceCount()
		assert.Len(t, evt.Application.Status.Resources, count)
	}
}

func TestCache_MissingClassErrors(t *testing.T) {
	c, err := NewCache(1, nil, SizeSmall)
	require.NoError(t, err)
	_, err = c.Get(SizeLarge)
	assert.Error(t, err)
}

func TestCacheWithPool_RotatesThroughDistinctPayloads(t *testing.T) {
	// Small pool size makes the test fast and easy to assert against.
	c, err := NewCacheWithPool(7, 4, nil, SizeSmall)
	require.NoError(t, err)
	require.Equal(t, 4, c.PoolSize(SizeSmall))

	// Pull pool entries; each should be byte-distinct (different UID + name
	// because the seed shifts per entry).
	first := make(map[string]struct{})
	for range 4 {
		js, err := c.Get(SizeSmall)
		require.NoError(t, err)
		first[js] = struct{}{}
	}
	assert.Len(t, first, 4, "all four pool entries must be distinct")

	// Cursor wraps: the next four pulls must repeat the first four (in order).
	second, err := c.Get(SizeSmall)
	require.NoError(t, err)
	_, ok := first[second]
	assert.True(t, ok, "after wrap, Get must return one of the original pool entries")
}

func TestCacheWithPool_RejectsZeroPoolSize(t *testing.T) {
	_, err := NewCacheWithPool(1, 0, nil, SizeSmall)
	assert.Error(t, err)
}
