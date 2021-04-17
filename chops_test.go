package chops

import (
	"reflect"
	"testing"
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
