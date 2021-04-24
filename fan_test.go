package chops

import (
	"testing"
	"time"

	"go.uber.org/goleak"
)

func sleepSendClose(ch chan interface{}, val interface{}, delay time.Duration) {
	time.Sleep(delay)
	ch <- val
	close(ch)
}

func TestMakeFanIn(t *testing.T) {
	tests := []struct {
		name   string
		outCap int
		chs    func() []interface{}
		post   func(t *testing.T, out chan interface{}, stop chan struct{})
	}{
		{
			"Two",
			2,
			func() []interface{} {
				ch0 := make(chan interface{})
				ch1 := make(chan interface{})
				go sleepSendClose(ch0, "Hello", time.Second)
				go sleepSendClose(ch1, "Goodbye", 2*time.Second)
				return []interface{}{ch0, ch1}
			},
			func(t *testing.T, out chan interface{}, stop chan struct{}) {
				if v := <-out; v != "Hello" {
					t.Fatalf("0: expected \"Hello\", got %q", v)
				}
				if v := <-out; v != "Goodbye" {
					t.Fatalf("1: expected \"Goodbye\", got %q", v)
				}
				v, ok := <-out
				if ok {
					t.Fatalf("expected closed ch, got %v", v)
				}
			},
		},
		{
			"Two closed",
			2,
			func() []interface{} {
				ch0 := make(chan interface{}, 1)
				ch1 := make(chan interface{}, 1)
				go sleepSendClose(ch0, "Hello", time.Second)
				go sleepSendClose(ch1, "Goodbye", 2*time.Second)
				return []interface{}{ch0, ch1}
			},
			func(t *testing.T, out chan interface{}, stop chan struct{}) {
				if v := <-out; v != "Hello" {
					t.Fatalf("0: expected \"Hello\", got %q", v)
				}
				close(stop)
				v, ok := <-out
				if ok {
					t.Fatalf("expected closed ch, got %v", v)
				}
			},
		},
		{
			"Zero",
			1,
			func() []interface{} {
				return nil
			},
			func(t *testing.T, out chan interface{}, stop chan struct{}) {
				v, ok := <-out
				if ok {
					t.Fatalf("expected closed ch, got %v", v)
				}
			},
		},
		{
			"One",
			2,
			func() []interface{} {
				ch := make(chan interface{}, 1)
				go func() {
					ch <- "Hello"
					time.Sleep(time.Second)
					ch <- "Goodbye"
					close(ch)
				}()
				return []interface{}{ch}
			},
			func(t *testing.T, out chan interface{}, stop chan struct{}) {
				if v := <-out; v != "Hello" {
					t.Fatalf("0: expected \"Hello\", got %q", v)
				}
				if v := <-out; v != "Goodbye" {
					t.Fatalf("1: expected \"Goodbye\", got %q", v)
				}
				v, ok := <-out
				if ok {
					t.Fatalf("expected closed ch, got %v", v)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, stop := MakeFanIn(tt.outCap, tt.chs()...)
			tt.post(t, out, stop)
			goleak.VerifyNone(t, goleak.IgnoreTopFunction("time.Sleep"))
		})
	}

}

func Test_MakeFanOut(t *testing.T) {
	tests := []struct {
		name   string
		n      int
		outCap int
		ch     func() interface{}
		post   func(*testing.T, []chan interface{}, interface{})
	}{
		{
			"Zero",
			0,
			0,
			func() interface{} {
				return make(chan struct{})
			},
			func(t *testing.T, out []chan interface{}, _ interface{}) {
				if len(out) != 0 {
					t.Fatalf("expected empty out, got %v", out)
				}
			},
		},
		{
			"One",
			1,
			1,
			func() interface{} {
				c := make(chan string)
				go func() {
					c <- "Hello"
					close(c)
				}()
				return c
			},
			func(t *testing.T, out []chan interface{}, _ interface{}) {
				if len(out) != 1 {
					t.Fatalf("expected len(out) == 1, got %v", out)
				}
				recvd := <-out[0]
				if recvd != "Hello" {
					t.Fatalf("expected \"Hello\", got %v", recvd)
				}
				recvd, ok := <-out[0]
				if ok {
					t.Fatalf("expected closed channel, received %v", recvd)
				}
			},
		},
		{
			"Two blocking",
			2,
			0,
			func() interface{} {
				c := make(chan string)
				go func() {
					c <- "Hello"
					c <- "Goodbye"
					close(c)
				}()
				return c
			},
			func(t *testing.T, out []chan interface{}, c interface{}) {
				if len(out) != 2 {
					t.Fatalf("expected len(out) == 2, got %v", out)
				}
				var recvd interface{}
				var status Status
				var ok bool

				recvd = <-out[0]
				if recvd != "Hello" {
					t.Fatalf("expected \"Hello\", got %v", recvd)
				}

				recvd, status = TryRecv(out[0])
				if status != Blocked || recvd != nil {
					t.Fatalf("expected blocked receive, got %v (%s)", recvd, status)
				}

				recvd = <-out[1]
				if recvd != "Hello" {
					t.Fatalf("expected \"Hello\", got %v", recvd)
				}

				recvd, status = TryRecv(out[1])
				if status != Blocked || recvd != nil {
					t.Fatalf("expected blocked receive, got %v (%s)", recvd, status)
				}

				recvd = <-out[0]
				if recvd != "Goodbye" {
					t.Fatalf("expected \"Goodbye\", got %v", recvd)
				}

				recvd = <-out[1]
				if recvd != "Goodbye" {
					t.Fatalf("expected \"Goodbye\", got %v", recvd)
				}

				// can't use TryRecv here, must wait for broadcaster to close
				recvd, ok = <-out[0]
				if ok || recvd != nil {
					t.Fatalf("expected closed channel, got %v", recvd)
				}

				recvd, ok = <-out[1]
				if ok || recvd != nil {
					t.Fatalf("expected closed channel, got %s", recvd)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.ch()
			out := MakeFanOut(tt.n, tt.outCap, c)
			tt.post(t, out, c)
			goleak.VerifyNone(t, goleak.IgnoreTopFunction("time.Sleep"))
		})
	}
}
