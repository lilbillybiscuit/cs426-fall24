package lab0

import (
	"context"
	"sync"
)

// MergeChannels should read from the channels `a` and `b`
// concurrently and write all values received to `out` as
// they are received.
//
// MergeChannels should run until all elements have been read from
// both `a` and `b`, then close `out` to signal that all results
// have been merged.
//
// The input parameters are guaranteed to be not `nil`.
//
// There are multiple ways to implement this method, any of which
// are valid as long as they meet the specification.
// If you are stuck, consider revisiting channels in Tour of Go:
//   - https://go.dev/tour/concurrency/4
//   - https://go.dev/tour/concurrency/5
func MergeChannels[T any](a <-chan T, b <-chan T, out chan<- T) {
	var wg sync.WaitGroup

	merge_func := func(c <-chan T, wg *sync.WaitGroup) {
		defer wg.Done()
		for {
			select {
			case val, ok := <-c:
				{
					if !ok {
						return
					}
					out <- val
				}
			}
		}
	}

	wg.Add(2)
	go merge_func(a, &wg)
	go merge_func(b, &wg)

	go func() {
		wg.Wait()
		close(out)
	}()

}

// MergeChannelsOrCancel provides similar semantics to MergeChannels, but
// allows for the caller to cancel processing by cancelling the context `ctx`.
// Results from channels `a` and `b` should be read concurrently and written
// to `out` until there are no more results in either channel, *or* `ctx` is
// done. If `ctx` is done and contains an error, it should be returned. In
// all other cases, `nil` should be returned.
//
// The input parameters are guaranteed to be not `nil`.
//
// For more details, read about contexts:
//   - https://pkg.go.dev/context
//   - https://www.digitalocean.com/community/tutorials/how-to-use-contexts-in-go#determining-if-a-context-is-done
//
// If the return value is confusing, read more about errors:
//   - https://go.dev/tour/methods/19
//
// It is expected that your implemented is similar to `MergeChannels`. You do
// not need to refactor to deduplicate your code, but you can if you want to.
func MergeChannelsOrCancel[T any](ctx context.Context, a <-chan T, b <-chan T, out chan<- T) error {
	// slightly less parallel but should still work
	for {
		var valid bool = false
		select {
		case res, ok := <-a:
			{
				if ok {
					select {
					case out <- res:
					default:
					}
					valid = true
				}
			}
		case res, ok := <-b:
			{
				if ok {
					select {
					case out <- res:
					default:
					}
					valid = true
				}
			}
		case <-ctx.Done():
			{
				close(out) // clarified on edstem
				return ctx.Err()
			}
		}
		if !valid {
			break
		}
	}
	close(out)
	return nil
}

// Fetcher is an interface which mimics fetching from some source
// like a database, web service, or file system. Fetching could take
// considerable time.
//
// Fetch() should be called multiple times to keep fetching new data.
// Fetching is considered done once `false` is returned.
//
// You do not need to implement `Fetcher` in any way, just use the
// `Fetch()` method as part of `MergeFetches`.
type Fetcher interface {

	// Fetch returns two values:
	//  - new data and `true` when there is data available to be fetched
	//  - "" and `false` when fetching is done
	//
	// Fetch() should not be called again once `false` is returned
	//
	// For example, fetching all data from a fetcher:
	// ```
	// for {
	//     data, ok := fetcher.Fetch()
	//     if !ok {
	//         break
	//     }
	//     fmt.Println("data: " + data)
	// }
	// ```
	Fetch() (string, bool)
}

// MergeFetches is similar to `MergeChannels`, however you must merge results
// returned from a "Fetcher" instead of a channel. Consider Fetcher like an
// interface for fetching data from a database or web service. It may take
// significant amount of time.
//
// MergeFetches must fetch from both `a` and `b` concurrently and write results
// to `out` until both fetchers are "done" (have returned `false` from `Fetch()`).
// Once complete, `out` must be closed.
//
// We recommend using `sync.WaitGroup` and goroutines to implement `MergeFetches`.
// If you are stuck, consider reading the example for `WaitGroup` here:
//   - https://pkg.go.dev/sync#example-WaitGroup
func MergeFetches(a Fetcher, b Fetcher, out chan<- string) {
	var wg sync.WaitGroup

	merge_fetcher := func(f Fetcher, out chan<- string) {
		defer wg.Done()
		for {
			res, ok := f.Fetch()
			if !ok {
				return
			}
			out <- res
		}
	}

	wg.Add(2)
	go merge_fetcher(a, out)
	go merge_fetcher(b, out)

	go func() {
		wg.Wait()
		close(out)
	}()
}
