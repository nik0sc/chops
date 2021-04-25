package chops

import (
	"reflect"
	"testing"
	"time"
)

func TestIsClosed(t *testing.T) {
	tests := []struct {
		name      string
		chFactory func() interface{}
		want      bool
	}{
		{
			"Open",
			func() interface{} {
				return make(chan struct{})
			},
			false,
		},
		{
			"Closed",
			func() interface{} {
				ch := make(chan struct{})
				close(ch)
				return ch
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsClosed(tt.chFactory()); got != tt.want {
				t.Errorf("IsClosed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTryRecv(t *testing.T) {
	tests := []struct {
		name      string
		chFactory func() interface{}
		want      interface{}
		want1     Status
	}{
		{
			"Ok",
			func() interface{} {
				ch := make(chan string, 1)
				ch <- "Hello"
				return ch
			},
			"Hello",
			Ok,
		},
		{
			"Closed",
			func() interface{} {
				ch := make(chan string)
				close(ch)
				return ch
			},
			"",
			Closed,
		},
		{
			"Blocked",
			func() interface{} {
				return make(chan struct{})
			},
			nil,
			Blocked,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := TryRecv(tt.chFactory())
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TryRecv() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("TryRecv() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestTrySend(t *testing.T) {
	tests := []struct {
		name      string
		chFactory func() interface{}
		x         interface{}
		wantStat  Status
	}{
		{
			"Ok",
			func() interface{} {
				return make(chan string, 1)
			},
			"Hello",
			Ok,
		},
		{
			"Closed",
			func() interface{} {
				ch := make(chan string)
				close(ch)
				return ch
			},
			"yeet",
			Closed,
		},
		{
			"Blocked",
			func() interface{} {
				return make(chan string)
			},
			"oof",
			Blocked,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotStat := TrySend(tt.chFactory(), tt.x); gotStat != tt.wantStat {
				t.Errorf("TrySend() = %v, want %v", gotStat, tt.wantStat)
			}
		})
	}
}

func TestRecvOr(t *testing.T) {
	tests := []struct {
		name      string
		chFactory func() interface{}
		f         func()
		fFactory  func() func()
		want      interface{}
		wantOk    bool
	}{
		{
			"Once, immediate receive",
			func() interface{} {
				ch := make(chan string, 1)
				ch <- "Hello"
				return ch
			},
			func() {
				t.Fatal("f() should not run")
			},
			nil,
			"Hello",
			true,
		},
		{
			"Once closed, immediate receive",
			func() interface{} {
				ch := make(chan struct{})
				close(ch)
				return ch
			},
			func() {
				t.Fatal("f() should not run")
			},
			nil,
			struct{}{}, // zero value of struct{}
			false,
		},
		{
			"Once, delay",
			func() interface{} {
				ch := make(chan string)
				time.AfterFunc(time.Second, func() {
					ch <- "Hello"
				})
				return ch
			},
			nil,
			func() func() {
				runCount := 0
				return func() {
					if runCount != 0 {
						t.Fatalf("f() ran too many times, expecting 0 and got %d", runCount)
					}
					t.Log("f() run")
					time.Sleep(2 * time.Second)
					runCount++
				}
			},
			"Hello",
			true,
		},
		{
			"Once closed, delay",
			func() interface{} {
				ch := make(chan struct{})
				time.AfterFunc(time.Second, func() {
					close(ch)
				})
				return ch
			},
			nil,
			func() func() {
				runCount := 0
				return func() {
					if runCount != 0 {
						t.Fatalf("f() ran too many times, expecting 0 and got %d", runCount)
					}
					t.Log("f() run")
					time.Sleep(2 * time.Second)
					runCount++
				}
			},
			struct{}{},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f func()
			if tt.fFactory != nil {
				f = tt.fFactory()
			} else {
				f = tt.f
			}

			got, ok := RecvOr(tt.chFactory(), f)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Received %v, want %v", got, tt.want)
			}
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func TestSendOr(t *testing.T) {
	tests := []struct {
		name      string
		chFactory func() interface{}
		x         interface{}
		f         func()
		wantOk    bool
		postcond  func(*testing.T, interface{})
	}{
		{
			"Send immediate",
			func() interface{} {
				return make(chan string, 1)
			},
			"Hello",
			func() {
				t.Fatal("f() should not run")
			},
			true,
			func(t *testing.T, ch interface{}) {
				recvd, ok := <-ch.(chan string)
				if !ok || recvd != "Hello" {
					t.Fatalf("expected to receive \"Hello\", got %v (%v)", recvd, ok)
				}
			},
		},
		{
			"Send blocked, run f",
			func() interface{} {
				ch := make(chan string)
				time.AfterFunc(time.Millisecond, func() {
					t.Logf("received: %s", <-ch)
				})
				return ch
			},
			"Hello",
			func() {
				t.Log("f() run")
			},
			true,
			func(t *testing.T, ch interface{}) {

			},
		},
		{
			"Send on closed channel, immediate",
			func() interface{} {
				ch := make(chan struct{})
				close(ch)
				return ch
			},
			struct{}{},
			func() {
				t.Fatal("f() should not run")
			},
			false,
			func(t *testing.T, ch interface{}) {
				if !IsClosed(ch) {
					t.Fatal("ch still open")
				}
			},
		},
		{
			"Send func immediate",
			func() interface{} {
				return make(chan string, 1)
			},
			func() string {
				return "Hello"
			},
			func() {
				t.Fatal("f() should not run")
			},
			true,
			func(t *testing.T, ch interface{}) {
				recvd, ok := <-ch.(chan string)
				if !ok || recvd != "Hello" {
					t.Fatalf("expected to receive \"Hello\", got %v (%v)", recvd, ok)
				}
			},
		},
		{
			"Send func blocked, run f",
			func() interface{} {
				ch := make(chan string)
				time.AfterFunc(time.Millisecond, func() {
					t.Logf("received: %s", <-ch)
				})
				return ch
			},
			func() string {
				return "Hello"
			},
			func() {
				t.Log("f() run")
			},
			true,
			func(t *testing.T, ch interface{}) {

			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.chFactory()
			ok := SendOr(c, tt.x, tt.f)
			if ok != tt.wantOk {
				t.Errorf("SendOr() = %v, want %v", ok, tt.wantOk)
			}
			tt.postcond(t, c)
		})
	}
}
