//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"
)

type singleInstanceLock struct {
	handle syscall.Handle
}

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex = kernel32.NewProc("CreateMutexW")
)

const errorAlreadyExists syscall.Errno = 183

func acquireSingleInstanceLock() (*singleInstanceLock, bool, error) {
	name, err := syscall.UTF16PtrFromString(`Global\jczhl-filyme-launcher-single-instance`)
	if err != nil {
		return nil, false, err
	}

	handle, _, callErr := procCreateMutex.Call(
		0,
		0,
		uintptr(unsafe.Pointer(name)),
	)
	if handle == 0 {
		if callErr != syscall.Errno(0) {
			return nil, false, callErr
		}
		return nil, false, errors.New("CreateMutexW failed")
	}

	lock := &singleInstanceLock{handle: syscall.Handle(handle)}
	if callErr == errorAlreadyExists {
		lock.Release()
		return nil, false, nil
	}

	return lock, true, nil
}

func (l *singleInstanceLock) Release() {
	if l == nil || l.handle == 0 {
		return
	}
	syscall.CloseHandle(l.handle)
	l.handle = 0
}
