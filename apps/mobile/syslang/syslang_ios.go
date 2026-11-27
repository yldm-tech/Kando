package syslang

import (
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// Keep is called with the language the app is about to force (Scope.SetLocale), "" to follow the system, before it does: it keeps the language the user chose for the app in iOS Settings aside while the app's own stands in its place, and puts it back as the app follows the system again. NSUserDefaults may be used from any thread.
func Keep(lang string) {
	// The engine's goroutine has no autorelease pool of its own, and the pool is the thread's.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(sel("new"))
	defer pool.Send(sel("drain"))
	keep(userDefaults{objc.ID(objc.GetClass("NSUserDefaults")).Send(sel("standardUserDefaults"))}, lang)
}

// userDefaults is the app's NSUserDefaults.
type userDefaults struct{ id objc.ID }

func (u userDefaults) strings(key string) []string {
	list := u.id.Send(sel("stringArrayForKey:"), nsString(key))
	if list == 0 {
		return nil
	}
	n := objc.Send[uint](list, sel("count"))
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, goString(list.Send(sel("objectAtIndex:"), i)))
	}
	return out
}

func (u userDefaults) string(key string) string {
	return goString(u.id.Send(sel("stringForKey:"), nsString(key)))
}

func (u userDefaults) setStrings(key string, v []string) {
	list := objc.ID(objc.GetClass("NSMutableArray")).Send(sel("array"))
	for _, s := range v {
		list.Send(sel("addObject:"), nsString(s))
	}
	u.id.Send(sel("setObject:forKey:"), list, nsString(key))
}

func (u userDefaults) remove(key string) { u.id.Send(sel("removeObjectForKey:"), nsString(key)) }

func sel(name string) objc.SEL { return objc.RegisterName(name) }

// nsString is s as an autoreleased NSString.
func nsString(s string) objc.ID {
	b := append([]byte(s), 0)
	ns := objc.ID(objc.GetClass("NSString")).Send(sel("stringWithUTF8String:"), &b[0])
	runtime.KeepAlive(b)
	return ns
}

// goString is the Go string of an NSString, "" for nil.
func goString(ns objc.ID) string {
	if ns == 0 {
		return ""
	}
	p := objc.Send[*byte](ns, sel("UTF8String"))
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}
