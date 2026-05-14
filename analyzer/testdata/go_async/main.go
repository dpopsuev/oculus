package main

func produce(ch chan<- int) {
	ch <- 42 // channel_send → ch
}

func consume(ch <-chan int) int {
	return <-ch // channel_recv → ch
}

// Run is the exported entry point; the walk starts here.
func Run() {
	ch := make(chan int, 1)
	go produce(ch) // goroutine → produce
	_ = consume(ch)
}

func main() { Run() }
