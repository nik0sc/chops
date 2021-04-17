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
			goleak.VerifyNone(t)
		})
	}

}
