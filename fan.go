package chops

import (
	"reflect"
)

// MakeFanIn constructs a new channel with capacity outCap
// and an aggregating goroutine. The aggregating goroutine
// listens for incoming values over the input channels and
// sends them on the out channel. When all the input
// channels have been closed, the output channel will be
// closed and the aggregating goroutine will exit.
//
// It is always safe to close the stop channel. After
// closing the stop channel, the aggregating goroutine
// will have exited. Do not send on the stop channel.
func MakeFanIn(outCap int, chs ...interface{}) (out chan interface{}, stop chan struct{}) {
	if len(chs) == 0 {
		out = make(chan interface{})
		close(out)
		stop = make(chan struct{})
		return
	}

	if len(chs) == 1 {
		ch := chs[0].(chan interface{})
		out = make(chan interface{}, outCap)
		stop = make(chan struct{})
		go func() {
			defer close(out)
			for {
				select {
				case <-stop:
					return
				case v, ok := <-ch:
					if !ok {
						return
					}
					out <- v
				}
			}
		}()
		return
	}

	out = make(chan interface{}, outCap)
	stop = make(chan struct{}, 1)

	cases := make([]reflect.SelectCase, len(chs)+1)
	for i, ifacev := range chs {
		cases[i] = reflect.SelectCase{
			Chan: assertChanValue(ifacev),
			Dir:  reflect.SelectRecv,
		}
	}
	cases[len(cases)-1] = reflect.SelectCase{
		Chan: assertChanValue(stop),
		Dir:  reflect.SelectRecv,
	}

	go func() {
		defer close(out)
		// don't count stop case
		remaining := len(chs)

		for {
			chosen, recv, ok := reflect.Select(cases)
			if chosen == len(cases)-1 && !ok {
				// stop channel was closed
				return
			} else if ok {
				out <- recv.Interface()
			} else if remaining == 1 {
				// last one to leave turns out the lights
				return
			} else {
				// avoids slice buffer reallocation
				cases[chosen].Chan = reflect.Value{}
				remaining--
			}
		}
	}()

	return
}

// MakeFanOut constructs a slice of n channels each with
// outCap capacity and a broadcasting goroutine. The
// goroutine listens for values on the input channel and
// broadcasts copies onto the output channels. When the
// input channel is closed, the output channels will all be
// closed and the broadcasting goroutine will exit.
func MakeFanOut(n int, outCap int, ch interface{}) []chan interface{} {
	// Can't have `ch chan interface{}` because channel types
	// are not covariant wrt their elements.
	v := assertChanValue(ch)
	if n == 0 {
		return nil
	}

	if n == 1 {
		out := make(chan interface{}, outCap)
		go func() {
			for {
				x, ok := v.Recv()
				if !ok {
					close(out)
					return
				}
				out <- x.Interface()
			}
		}()
		return []chan interface{}{out}
	}

	out := make([]chan interface{}, n)
	for i := range out {
		out[i] = make(chan interface{}, outCap)
	}

	go func(v reflect.Value, out []chan interface{}) {
		// This will stall if goroutines are blocked up
		// Solution is to increase outCap?
		for {
			x, ok := v.Recv()
			if ok {
				for _, v := range out {
					v <- x.Interface()
				}
			} else {
				for _, v := range out {
					close(v)
				}
				return
			}
		}
	}(v, out)

	return out
}
