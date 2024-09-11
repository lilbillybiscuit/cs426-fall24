package lab0_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"cs426.cloud/lab0"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func chanToSlice[T any](ch chan T) []T {
	vals := make([]T, 0)
	for item := range ch {
		vals = append(vals, item)
	}
	return vals
}

type mergeFunc = func(chan string, chan string, chan string)

func runMergeTest(t *testing.T, merge mergeFunc) {
	t.Run("empty channels", func(t *testing.T) {
		a := make(chan string)
		b := make(chan string)
		out := make(chan string)
		close(a)
		close(b)

		merge(a, b, out)
		// If your lab0 hangs here, make sure you are closing your channels!
		require.Empty(t, chanToSlice(out))
	})

	// Please write your own tests
}

func TestMergeChannels(t *testing.T) {
	runMergeTest(t, func(a, b, out chan string) {
		lab0.MergeChannels(a, b, out)
	})
}

func TestMergeOrCancel(t *testing.T) {
	runMergeTest(t, func(a, b, out chan string) {
		_ = lab0.MergeChannelsOrCancel(context.Background(), a, b, out)
	})

	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		a := make(chan string, 1)
		b := make(chan string, 1)
		out := make(chan string, 10)

		eg, _ := errgroup.WithContext(context.Background())
		eg.Go(func() error {
			return lab0.MergeChannelsOrCancel(ctx, a, b, out)
		})
		err := eg.Wait()
		a <- "a"
		b <- "b"

		require.Error(t, err)
		require.Equal(t, []string{}, chanToSlice(out))
	})

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		a := make(chan string)
		b := make(chan string)
		out := make(chan string, 10)

		eg, _ := errgroup.WithContext(context.Background())
		eg.Go(func() error {
			return lab0.MergeChannelsOrCancel(ctx, a, b, out)
		})
		a <- "a"
		b <- "b"
		cancel()

		err := eg.Wait()
		require.Error(t, err)
		require.Equal(t, []string{"a", "b"}, chanToSlice(out))
	})
}

type channelFetcher struct {
	ch chan string
}

func newChannelFetcher(ch chan string) *channelFetcher {
	return &channelFetcher{ch: ch}
}

func (f *channelFetcher) Fetch() (string, bool) {
	v, ok := <-f.ch
	return v, ok
}

func TestMergeFetches(t *testing.T) {
	runMergeTest(t, func(a, b, out chan string) {
		lab0.MergeFetches(newChannelFetcher(a), newChannelFetcher(b), out)
	})
}

func TestMergeFetchesAdditional(t *testing.T) {
	t.Run("fetchers with data and output blocking", func(t *testing.T) {
		// this test takes advantage of the fact that my fetchers run concurrently in goroutines
		// so we try to load as much as possible
		a := make(chan string, 2)
		b := make(chan string, 2)
		out := make(chan string, 10)

		pusher := func(a chan string, digit string) {
			for _ = range 1000 {
				a <- digit
			}
			close(a)
		}

		go pusher(a, "1")
		go pusher(b, "2")

		go lab0.MergeFetches(newChannelFetcher(a), newChannelFetcher(b), out)

		// make a list of 1000 "1" and 1000 "2"
		expected := make([]string, 2000)
		for i := 0; i < 1000; i++ {
			expected[i] = "1"
			expected[i+1000] = "2"
		}
		result := make([]string, 0)
		var wg sync.WaitGroup
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			for {
				select {
				case v, ok := <-out:
					if !ok {
						return
					} else {
						result = append(result, v)
					}
				}
			}
		}(&wg)

		wg.Wait()
		// sort the result
		sort.Strings(result)

		require.ElementsMatch(t, expected, result)
	})

	t.Run("one fetcher empty", func(t *testing.T) {
		a := make(chan string, 3)
		b := make(chan string)
		out := make(chan string, 10)

		a <- "a1"
		a <- "a2"
		a <- "a3"
		close(a)
		close(b)

		go lab0.MergeFetches(newChannelFetcher(a), newChannelFetcher(b), out)

		expected := []string{"a1", "a2", "a3"}
		actual := chanToSlice(out)
		require.ElementsMatch(t, expected, actual)
	})

	t.Run("fetchers with delay", func(t *testing.T) {
		a := make(chan string, 3)
		b := make(chan string, 3)
		out := make(chan string, 10)

		go func() {
			time.Sleep(100 * time.Millisecond)
			a <- "a1"
			time.Sleep(100 * time.Millisecond)
			a <- "a2"
			time.Sleep(100 * time.Millisecond)
			a <- "a3"
			close(a)
		}()

		go func() {
			time.Sleep(150 * time.Millisecond)
			b <- "b1"
			time.Sleep(150 * time.Millisecond)
			b <- "b2"
			time.Sleep(150 * time.Millisecond)
			b <- "b3"
			close(b)
		}()

		go lab0.MergeFetches(newChannelFetcher(a), newChannelFetcher(b), out)

		expected := []string{"a1", "a2", "a3", "b1", "b2", "b3"}
		actual := chanToSlice(out)
		require.ElementsMatch(t, expected, actual)
	})
}

func TestMergeChannelsAdditional(t *testing.T) {
	t.Run("channels with data and output blocking", func(t *testing.T) {
		a := make(chan string, 1000)
		b := make(chan string, 1000)
		out := make(chan string, 2000)

		pusher := func(ch chan string, digit string) {
			for i := 0; i < 1000; i++ {
				ch <- digit
			}
			close(ch)
		}

		go pusher(a, "1")
		go pusher(b, "2")

		go lab0.MergeChannels(a, b, out)

		expected := make([]string, 2000)
		for i := 0; i < 1000; i++ {
			expected[i] = "1"
			expected[i+1000] = "2"
		}
		result := chanToSlice(out)

		sort.Strings(result)
		require.ElementsMatch(t, expected, result)
	})

}
