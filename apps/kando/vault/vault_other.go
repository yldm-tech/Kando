//go:build !darwin

package vault

import "context"

// Supported reports whether a library's files can be kept in an encrypted image here: only macOS, whose hdiutil makes and attaches them. Elsewhere a library's password is the app lock alone.
func Supported() bool { return false }

// Mounted reports whether a library's image is attached over its folder; there is none here.
func Mounted(string) bool { return false }

// RootOf is the folder of the open library whose image holds path; there is none here.
func RootOf(string) (string, bool) { return "", false }

func Create(context.Context, string, string, Progress) error { return ErrUnsupported }

func Open(context.Context, string, string) error { return ErrUnsupported }

func Close(context.Context, string, bool) error { return nil }

func ChangePassword(context.Context, string, string, string) error { return ErrUnsupported }

func Remove(context.Context, string, string, Progress) error { return ErrUnsupported }
