//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <pthread.h>
extern void canveppSpaceEvent(int kind, long long stamp);
static pthread_mutex_t cvMutex = PTHREAD_MUTEX_INITIALIZER;
static CFRunLoopRef cvLoop = NULL;
static CFMachPortRef cvTap = NULL;
static int cvStopping = 0;
static CGEventRef cvEvent(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *context) {
 if (type == kCGEventTapDisabledByTimeout || type == kCGEventTapDisabledByUserInput) {
  if(cvTap) CGEventTapEnable(cvTap, true);
  canveppSpaceEvent(0, 0); return event;
 }
 int kind=0;
 if(type == kCGEventKeyDown || type == kCGEventKeyUp) {
  int space = CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode) == 49;
  int repeat = CGEventGetIntegerValueField(event, kCGKeyboardEventAutorepeat) != 0;
  CGEventFlags blocked = kCGEventFlagMaskCommand | kCGEventFlagMaskControl | kCGEventFlagMaskAlternate | kCGEventFlagMaskShift;
  if(space && !repeat && !(CGEventGetFlags(event) & blocked)) kind=type==kCGEventKeyDown?1:2;
 }
 canveppSpaceEvent(kind, (long long)CGEventGetTimestamp(event));
 return event; // listen-only: never suppress or inject user keystrokes
}
static int cvAllowed(){return CGPreflightListenEventAccess();}
static int cvRequest(){return CGRequestListenEventAccess();}
static int cvPrepare(){
 pthread_mutex_lock(&cvMutex);cvStopping=0;pthread_mutex_unlock(&cvMutex);return 1;
}
static int cvStart(){
 CGEventMask mask=CGEventMaskBit(kCGEventKeyDown)|CGEventMaskBit(kCGEventKeyUp)|CGEventMaskBit(kCGEventLeftMouseDown)|CGEventMaskBit(kCGEventRightMouseDown)|CGEventMaskBit(kCGEventOtherMouseDown);
 CFMachPortRef tap=CGEventTapCreate(kCGSessionEventTap,kCGHeadInsertEventTap,kCGEventTapOptionListenOnly,mask,cvEvent,NULL);
 if(!tap)return 0;
 CFRunLoopSourceRef src=CFMachPortCreateRunLoopSource(kCFAllocatorDefault,tap,0);
 CFRunLoopRef loop=CFRunLoopGetCurrent();
 pthread_mutex_lock(&cvMutex);cvLoop=loop;cvTap=tap;int stop=cvStopping;pthread_mutex_unlock(&cvMutex);
 CFRunLoopAddSource(loop,src,kCFRunLoopCommonModes);CGEventTapEnable(tap,true);
 canveppSpaceEvent(10, 0);
 if(!stop)CFRunLoopRun();
 pthread_mutex_lock(&cvMutex);cvLoop=NULL;cvTap=NULL;pthread_mutex_unlock(&cvMutex);
 CGEventTapEnable(tap,false);CFRunLoopRemoveSource(loop,src,kCFRunLoopCommonModes);CFRelease(src);CFRelease(tap);return 1;
}
static void cvStop(){pthread_mutex_lock(&cvMutex);cvStopping=1;if(cvLoop)CFRunLoopStop(cvLoop);pthread_mutex_unlock(&cvMutex);}
*/
import "C"

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type spaceMonitor struct {
	done     chan struct{}
	exited   chan struct{}
	stopping atomic.Bool
	once     sync.Once
}

var spaceEvents = make(chan struct {
	kind int
	at   time.Duration
}, 128)

//export canveppSpaceEvent
func canveppSpaceEvent(kind C.int, stamp C.longlong) {
	select {
	case spaceEvents <- struct {
		kind int
		at   time.Duration
	}{int(kind), time.Duration(stamp)}:
	default:
	}
}
func spacePermission() bool        { return C.cvAllowed() != 0 }
func requestSpacePermission() bool { return C.cvRequest() != 0 }
func startSpaceMonitor(trigger func(), ready func(), stopped func()) *spaceMonitor {
	if !spacePermission() {
		return nil
	}
	C.cvPrepare()
	m := &spaceMonitor{done: make(chan struct{}), exited: make(chan struct{})}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		traceShortcut("listener_starting", 0)
		result := C.cvStart()
		if result == 0 {
			traceShortcut("listener_start_failed", 0)
		} else {
			traceShortcut("listener_exited", 0)
		}
		close(m.exited)
		m.once.Do(func() { close(m.done) })
		if stopped != nil && !m.stopping.Load() {
			stopped()
		}
	}()
	go func() {
		seq := tripleSpace{}
		for {
			select {
			case <-m.done:
				return
			case ev := <-spaceEvents:
				switch ev.kind {
				case 10:
					traceShortcut("listener_ready", 0)
					if ready != nil {
						ready()
					}
					continue
				case 0:
					traceShortcut("sequence_reset", ev.at)
				case 1:
					traceShortcut("space_down", ev.at)
				case 2:
					traceShortcut("space_up", ev.at)
				}
				if seq.feed(ev.kind, ev.at) {
					traceShortcut("sequence_match", ev.at)
					trigger()
				}
			}
		}
	}()
	return m
}
func (m *spaceMonitor) Stop() {
	if m == nil {
		return
	}
	m.stopping.Store(true)
	m.once.Do(func() { close(m.done); C.cvStop() })
	<-m.exited
}
