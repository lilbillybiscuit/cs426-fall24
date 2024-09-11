package main

import (
	"fmt"
	"sync"
	"time"
)

type Bar struct{}
type Foo struct {
	items  []string
	str    string
	num    int
	barPtr *Bar
	bar    Bar
}

func FunWithStructs() {
	var foo Foo
	fmt.Println(foo.items)
	fmt.Println(len(foo.items))
	fmt.Println(foo.items == nil)
	fmt.Println(foo.str)
	fmt.Println(foo.num)
	fmt.Println(foo.barPtr)
	fmt.Println(foo.bar)
}
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
}
