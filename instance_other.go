//go:build !windows

package main

type singleInstanceLock struct{}

func acquireSingleInstanceLock() (*singleInstanceLock, bool, error) {
	return &singleInstanceLock{}, true, nil
}

func (l *singleInstanceLock) Release() {}
