1) An unbuffered channel doesn't hold anything, and will block on read if it hasn't been read. Therefore it requires both the sender and receiver to be ready at the same time. In a buffered channel, we can set the number of elements to keep in the channel, so we could push multiple elemenets into the channel before we start getting blocking.
2) Unbuffered (`c := make(chan int)` is default and unbuffered)
3) Results in a deadlock, there is no routine to recieve the message "hello world" and therefore it stops there.
4) `<- chan T` is recieve only, `chan <- T` is send-only, and `chan T` is bidirectional, all of type T
5) It will continue reading until the channel/buffer is empty. Once the buffer is empty, it will return the 0 value, and the read will not block. For a nil channel, it will block forever since `nil` is not operational
6) It terminates when the channel is closed and there are no more items in the channel
7) If `ctx, cancel := context.Background()` (or any other context type), to determine if a context.Context is done or canceled, we can use the ctx.Done() channel. This channel is closed when the context is canceled or the timeout/deadline expires. For example:
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

select {
case <-ctx.Done():
    fmt.Println("context is done:", ctx.Err())
case result := <-ch:
    fmt.Println("result:", result)
}

```
8) If this was the only function in the program, then very likely `all done!` all on newlines. This is because we have scheduled goroutines that print 1 in 1 second, 2 in 2 seconds, and 3 after 3 seconds, but the program terminates in less than a second, so Go will not wait for all the goroutines to finish before exiting.
9) sync.WaitGroup
10) A Mutex ensures exclusive access to a resource by only allowing one goroutine at a time, by providing the `Lock()` and `Unlock()` interfaces. This is useful for times where exactly one thing can access an item, and preventing race conditions. A semaphore is more like a capacity manager for limiting concurrent access to a pool of resources through the `Acquire()` and `Release()` methods. This is useful for things like limiting the number of concurrent connections to an API, like database connections or a rate limited API.
11) It will print
```[]
0
true

0
<nil>
{}
```

12) `struct{}` is a zero-byte data structure. `chan struct{}` means we're creating channel passing zero-byte messages, which is highly memory-efficient compared to passing some other random nonzero-size data structure through the channel. We might use it solely for signaling purposes, to send a mere event or notification. For example, we might use it to notify the parent that a task was completed:
```
func task(done chan struct{}) {
    // some task here
    done <- struct{}{}
}

func main() {
    done := make(chan struct{})
    go task(done)

    select {
    case <-done:
        fmt.Println("complete")
    case <-time.After(time.Second):
        fmt.Println("timed out")
    }
}
```

- Or we might use it to broadcast a stop signal to all workers:
```go
func worker(stop chan struct{}) {
    for {
        select {
        case <-stop:
            fmt.Println("stopping")
            return
        default:
            // some work
        }
    }
}

func main() {
    stop := make(chan struct{})
    go worker(stop)
    go worker(stop)
    time.Sleep(time.Second)

    close(stop) // stops all workers as there is no more data and channel is closed
    fmt.Println("stopped all workers")
}

```

13) In some versions of go (<=1.22) there is an issue where the variable i is not captured within the goroutine, so instead of printing `1 2 3` it might print `4 4 4` since the goroutine executes after `i` has iterated to 4 (and treated as if it were still in scope). However, if we put the variable `i` outside the for loop, then the goroutine executes with `4 4 4` even with version 1.23. Example code below:
```go
func main() {
	var wg sync.WaitGroup
	i := 1
	for ; i <= 3; i++ {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			time.Sleep(time.Duration(i) * time.Second)
			fmt.Printf("%d\n", i)
		}(&wg)
	}
	wg.Wait()
	fmt.Println("all done!")
}```
