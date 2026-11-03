//go:build darwin

package main

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// This file is overlaid onto MyGo's cmd/mygo by tools/mygo-dev/build.sh. It lets a reload start the new build before the running one quits: the new build counts as up once one of its windows is on screen, which CoreGraphics tells without the app's help.

const (
	cgWindowListOnScreenOnly = 1
	cfNumberSInt32Type       = 3
)

var cg struct {
	once sync.Once
	ok   bool

	copyWindowInfo func(option, relativeTo uint32) uintptr
	arrayCount     func(array uintptr) int
	arrayAt        func(array uintptr, index int) uintptr
	dictGet        func(dict, key uintptr) uintptr
	numberValue    func(number uintptr, typ int, out unsafe.Pointer) bool
	release        func(ref uintptr)

	ownerPID uintptr // kCGWindowOwnerPID
	layer    uintptr // kCGWindowLayer
}

func loadCG() bool {
	cg.once.Do(func() {
		core, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		pid, err := purego.Dlsym(core, "kCGWindowOwnerPID")
		if err != nil {
			return
		}
		layer, err := purego.Dlsym(core, "kCGWindowLayer")
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&cg.copyWindowInfo, core, "CGWindowListCopyWindowInfo")
		purego.RegisterLibFunc(&cg.arrayCount, cf, "CFArrayGetCount")
		purego.RegisterLibFunc(&cg.arrayAt, cf, "CFArrayGetValueAtIndex")
		purego.RegisterLibFunc(&cg.dictGet, cf, "CFDictionaryGetValue")
		purego.RegisterLibFunc(&cg.numberValue, cf, "CFNumberGetValue")
		purego.RegisterLibFunc(&cg.release, cf, "CFRelease")
		// The symbols are global CFStringRef variables in CoreGraphics, outside the Go heap: read the reference they hold.
		cg.ownerPID = **(**uintptr)(unsafe.Pointer(&pid))
		cg.layer = **(**uintptr)(unsafe.Pointer(&layer))
		cg.ok = true
	})
	return cg.ok
}

// windowShown reports whether the process pid has a normal window (layer 0) on screen.
func windowShown(pid int) bool {
	if !loadCG() {
		return false
	}
	list := cg.copyWindowInfo(cgWindowListOnScreenOnly, 0)
	if list == 0 {
		return false
	}
	defer cg.release(list)
	for i, n := 0, cg.arrayCount(list); i < n; i++ {
		info := cg.arrayAt(list, i)
		if cfInt(cg.dictGet(info, cg.ownerPID)) == int32(pid) && cfInt(cg.dictGet(info, cg.layer)) == 0 {
			return true
		}
	}
	return false
}

func cfInt(number uintptr) int32 {
	var v int32 = -1
	if number != 0 {
		cg.numberValue(number, cfNumberSInt32Type, unsafe.Pointer(&v))
	}
	return v
}

// windowProbe is nil where reloads keep MyGo's order: the running build quits, then the next starts.
var windowProbe = windowShown
