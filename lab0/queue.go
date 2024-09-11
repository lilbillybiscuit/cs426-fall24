package lab0

import "sync"

// Queue is a simple FIFO queue that is unbounded in size.
// Push may be called any number of times and is not
// expected to fail or overwrite existing entries.
type Queue[T any] struct {
	// Add your fields here
	frontIndex int
	curIndex   int
	size       int
	capacity   int
	arr        []T
}

func (q *Queue[T]) resize() {
	newCapacity := 2 * q.capacity
	newArr := make([]T, newCapacity)
	for i := 0; i < q.size; i++ {
		realIndex := (q.frontIndex + i) % q.capacity
		newArr[i] = q.arr[realIndex]
	}
	q.arr = newArr
	q.frontIndex = 0
	q.curIndex = q.size
	q.capacity = newCapacity
}

// NewQueue returns a new queue which is empty.
func NewQueue[T any]() *Queue[T] {
	q := &Queue[T]{
		size:       0,
		capacity:   4,
		arr:        make([]T, 4),
		frontIndex: 0,
		curIndex:   0,
	}
	return q
}

// Push adds an item to the end of the queue.
func (q *Queue[T]) Push(t T) {
	if q.size >= q.capacity {
		q.resize()
	}
	q.arr[q.curIndex] = t
	q.curIndex = (q.curIndex + 1) % q.capacity
	q.size++
}

// Pop removes an item from the beginning of the queue
// and returns it unless the queue is empty.
//
// If the queue is empty, returns the zero value for T and false.
//
// If you are unfamiliar with "zero values", consider revisiting
// this section of the Tour of Go: https://go.dev/tour/basics/12
func (q *Queue[T]) Pop() (T, bool) {
	if q.size == 0 {
		var temp T // default to a zero value
		return temp, false
	}
	item := q.arr[q.frontIndex]
	q.frontIndex = (q.frontIndex + 1) % q.capacity
	q.size--
	return item, true
}

// ConcurrentQueue provides the same semantics as Queue but
// is safe to access from many goroutines at once.
//
// You can use your implementation of Queue[T] here.
//
// If you are stuck, consider revisiting this section of
// the Tour of Go: https://go.dev/tour/concurrency/9
type ConcurrentQueue[T any] struct {
	// Add your fields here
	mu    sync.Mutex
	queue *Queue[T]
}

func NewConcurrentQueue[T any]() *ConcurrentQueue[T] {
	cq := &ConcurrentQueue[T]{
		queue: NewQueue[T](),
	}
	return cq
}

// Push adds an item to the end of the queue
func (q *ConcurrentQueue[T]) Push(t T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue.Push(t)
}

// Pop removes an item from the beginning of the queue.
// Returns a zero value and false if empty.
func (q *ConcurrentQueue[T]) Pop() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queue.Pop()
}
