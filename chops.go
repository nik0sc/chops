// Package chops provides useful channel operations
// that are not provided by the standard `<-` mechanism.
// It is not guaranteed to be compatible with all versions
// of Go, although it is tested on Go 1.16.
//
// Channels are often typed as `interface{}` when used as
// parameters in chops' functions. This is because Go does
// not support covariance on channels and their elements.
// As an example, `chan int` is not assignable to `chan
// interface{}`. The concrete type of `interface{}` is
// enforced at runtime instead.
package chops

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"unsafe"
)

// Status represents the result of a non-blocking channel
// operation. It can be Ok, Closed, or Blocked.
type Status int

func (s Status) String() string {
	switch s {
	case Ok:
		return "Ok"
	case Closed:
		return "Closed"
	case Blocked:
		return "Blocked"
	default:
		return "<invalid chops.Status>"
	}
}

const (
	// The channel accepted the send or receive without
	// blocking.
	Ok Status = iota
	// The channel is closed. Future operations will always
	// return Closed again.
	Closed
	// The channel is not ready to accept the operation.
	// Its buffer could be full, or if it's unbuffered, no
	// goroutine is waiting on the other end.
	Blocked
)

const closeChMsg = "send on closed channel"
const doubleCloseMsg = "close of closed channel"

// Warning: hackery here! Correct as of 1.16
type ifaceChan struct {
	_    uintptr
	data *struct {
		_      uint
		_      uint
		_      unsafe.Pointer
		_      uint16
		closed uint32
	}
}

// This is extra important for IsClosed
func assertChanValue(ch interface{}) reflect.Value {
	v := reflect.ValueOf(ch)
	if v.Kind() != reflect.Chan {
		panic(fmt.Sprintf("not a channel: %T", ch))
	}
	return v
}

// TryRecv attempts a non-blocking receive from a channel.
// It wraps the (reflect.Value).TryRecv method.
// If the return Status is Ok, the receive succeeded and
// the return interface{} may be asserted.
// If the return Status is Closed, the channel is closed
// and the return interface{} will be the zero value of
// the channel's element type.
// If the return Status is Blocked, the channel is empty
// (but not closed, at the time of the receive) and the
// return interface{} will be nil.
func TryRecv(ch interface{}) (interface{}, Status) {
	v := assertChanValue(ch)
	x, ok := v.TryRecv()
	if ok {
		return x.Interface(), Ok
	} else if x.IsValid() {
		return x.Interface(), Closed
	} else {
		return nil, Blocked
	}
}

// TrySend attempts a non-blocking send to a channel.
// It wraps the (reflect.Value).TrySend method.
// If the return Status is Ok, the send succeeded.
// If the return Status is Closed, the channel is closed.
// Future calls to TrySend will continue to return Closed.
// If the return Status is Blocked, the channel is either
// full (if it is buffered) or nobody is listening on the
// other end (if it is unbuffered).
func TrySend(ch interface{}, x interface{}) (stat Status) {
	v := assertChanValue(ch)
	xt := reflect.TypeOf(x)
	if !xt.AssignableTo(v.Type().Elem()) {
		panic(fmt.Sprintf("cannot send %T on %T", x, ch))
	}

	defer func() {
		r := recover()
		if r == nil {
			return
		}
		err, ok := r.(runtime.Error)
		if ok && strings.Contains(err.Error(), closeChMsg) {
			stat = Closed
		} else {
			panic(r)
		}
	}()

	if v.TrySend(reflect.ValueOf(x)) {
		stat = Ok
	} else {
		stat = Blocked
	}
	return
}

// TryClose ensures a channel is closed. It returns true
// if the channel was previously open, or false if the
// channel was already closed at the time of the call.
//
// You should not need to use this function normally, since
// only the sender should close a channel. A suggested
// application is a multi-producer situation where you
// want to avoid a secondary `closed chan struct{}`.
func TryClose(ch interface{}) (ok bool) {
	v := assertChanValue(ch)
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		err, ok := r.(runtime.Error)
		if ok && strings.Contains(err.Error(), doubleCloseMsg) {
			ok = false
		} else {
			panic(r)
		}
	}()
	v.Close()
	ok = true
	return
}

// IsClosed returns true if the channel provided is closed.
// You cannot assume that the channel is not closed if this
// function returns false. The channel may still contain
// data to be read, use `len()` to determine that. If the
// passed interface{} is not a channel type, IsClosed will
// panic.
func IsClosed(ch interface{}) bool {
	assertChanValue(ch)
	ifaceh := (*ifaceChan)(unsafe.Pointer(&ch))
	// This is technically wrong since channels have a mutex
	// within them to protect access. But closed never goes
	// from 1 back to 0, and we have some warnings in the
	// documentation about relying on false result (could be
	// the result of a dirty read).
	return ifaceh.data.closed == 1
}

// RecvOr attempts a non-blocking receive on a channel. It
// behaves like the two-result receive `x, ok := <-ch`, but
// if the receive is blocked, it will run the function f
// instead and try the non-blocking receive again after it
// returns, and so on until the receive succeeds.
//
// Use this instead of TryRecv in tight loops to avoid the
// overhead of boxing channels to interfaces in every loop
// iteration.
func RecvOr(ch interface{}, f func()) (interface{}, bool) {
	v := assertChanValue(ch)
	for {
		x, ok := v.TryRecv()
		if !x.IsValid() {
			f()
		} else {
			return x.Interface(), ok
		}
	}
}

// SendOr attempts a non-blocking send on a channel. It
// behaves like the standard `ch <- x`, but if the send is
// blocked, it will run the function f instead and try the
// non-blocking send again after it returns, and so on
// until the send succeeds. SendOr returns true if the send
// eventually succeeds, or false if the channel is closed.
//
// Use this instead of TrySend in tight loops to avoid the
// overhead of boxing channels to interfaces in every loop
// iteration.
func SendOr(ch interface{}, x interface{}, f func()) (ok bool) {
	v := assertChanValue(ch)
	xt := reflect.TypeOf(x)
	if !xt.AssignableTo(v.Type().Elem()) {
		panic(fmt.Sprintf("cannot send %T on %T", x, ch))
	}

	xv := reflect.ValueOf(x)
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		err, ok2 := r.(runtime.Error)
		if ok2 && strings.Contains(err.Error(), closeChMsg) {
			ok = false
		} else {
			panic(r)
		}
	}()
	for {
		ok = v.TrySend(xv)
		if ok {
			break
		}
		f()
	}
	return
}
