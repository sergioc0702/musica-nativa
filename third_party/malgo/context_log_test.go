package malgo

import (
	"testing"
	"time"
	"unsafe"
)

// Context.Devices holds contextMutex while miniaudio enumerates devices, and
// miniaudio may log from inside that call (for example when a device is busy).
// The log callback then runs on the same goroutine; it must not wait for
// contextMutex, or the enumeration deadlocks forever.
func TestLogDuringDeviceEnumerationDoesNotDeadlock(t *testing.T) {
	// Stands in for a ma_context: aligned and larger than the real struct, so
	// the pointer conversions pass the race detector's pointer checks.
	storage := make([]uint64, 1<<14)
	ptr := unsafe.Pointer(&storage[0])
	ctx := Context{ptr: &ptr}
	var logged []string
	ctx.SetLogProc(func(message string) { logged = append(logged, message) })

	done := make(chan struct{})
	go func() {
		contextMutex.Lock() // as Context.Devices does around ma_context_get_devices
		dispatchLog(ptr, "dispositivo ocupado")
		contextMutex.Unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		// No cleanup here: the stuck goroutine keeps contextMutex for good.
		t.Fatal("the log callback deadlocked while the context lock was held")
	}
	ctx.SetLogProc(nil)
	if len(logged) != 1 || logged[0] != "dispositivo ocupado" {
		t.Fatalf("logged = %q; want the one message", logged)
	}
}
