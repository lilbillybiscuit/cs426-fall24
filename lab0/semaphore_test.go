package lab0_test

import (
	"context"
	"testing"
	"time"

	"cs426.cloud/lab0"
	"github.com/stretchr/testify/require"
)

func TestSemaphore(t *testing.T) {
	t.Run("semaphore basic", func(t *testing.T) {
		s := lab0.NewSemaphore()
		go func() {
			s.Post()
		}()
		err := s.Wait(context.Background())
		require.NoError(t, err)
	})
	t.Run("semaphore starts with zero available resources", func(t *testing.T) {
		s := lab0.NewSemaphore()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		err := s.Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
	t.Run("semaphore post before wait does not block", func(t *testing.T) {
		s := lab0.NewSemaphore()
		s.Post()
		s.Post()
		s.Post()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		err := s.Wait(ctx)
		require.NoError(t, err)
	})
	t.Run("post after wait releases the wait", func(t *testing.T) {
		s := lab0.NewSemaphore()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		go func() {
			time.Sleep(20 * time.Millisecond)
			s.Post()
		}()
		err := s.Wait(ctx)
		require.NoError(t, err)
	})

	// Additional test cases

	t.Run("multiple waiters released in order", func(t *testing.T) {
		s := lab0.NewSemaphore()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		results := make(chan int, 3)
		for i := 0; i < 3; i++ {
			go func(i int) {
				err := s.Wait(ctx)
				require.NoError(t, err)
				results <- i
			}(i)
		}

		time.Sleep(20 * time.Millisecond)
		s.Post()
		s.Post()
		s.Post()

		for i := 0; i < 3; i++ {
			select {
			case res := <-results:
				require.Contains(t, []int{0, 1, 2}, res)
			case <-ctx.Done():
				t.Fatal("timed out waiting for results")
			}
		}
	})

	t.Run("multiple posts before wait", func(t *testing.T) {
		s := lab0.NewSemaphore()
		s.Post()
		s.Post()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		err := s.Wait(ctx)
		require.NoError(t, err)

		err = s.Wait(ctx)
		require.NoError(t, err)
	})

	t.Run("context cancellation", func(t *testing.T) {
		s := lab0.NewSemaphore()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		go func() {
			time.Sleep(20 * time.Millisecond)
			s.Post()
		}()

		err := s.Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("context done before wait", func(t *testing.T) {
		s := lab0.NewSemaphore()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := s.Wait(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("semaphore with multiple resources", func(t *testing.T) {
		s := lab0.NewSemaphore()
		for i := 0; i < 3; i++ {
			s.Post()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		for i := 0; i < 3; i++ {
			err := s.Wait(ctx)
			require.NoError(t, err)
		}
	})

	t.Run("semaphore with interleaved post and wait", func(t *testing.T) {
		s := lab0.NewSemaphore()

		go func() {
			time.Sleep(10 * time.Millisecond)
			s.Post()
		}()

		go func() {
			time.Sleep(20 * time.Millisecond)
			s.Post()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := s.Wait(ctx)
		require.NoError(t, err)

		err = s.Wait(ctx)
		require.NoError(t, err)
	})
}
