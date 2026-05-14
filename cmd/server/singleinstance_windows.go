//go:build windows

package main

import (
	"log"

	"golang.org/x/sys/windows"
)

const singletonName = `ZealClinic-singleton-lock`

func alreadyRunning() bool {
	name, err := windows.UTF16PtrFromString(singletonName)
	if err != nil {
		log.Printf("[main] single-instance check skipped: %v", err)
		return false
	}
	if _, err = windows.CreateMutex(nil, false, name); err == windows.ERROR_ALREADY_EXISTS {
		return true
	} else if err != nil {
		log.Printf("[main] single-instance check skipped: %v", err)
	}
	return false
}
