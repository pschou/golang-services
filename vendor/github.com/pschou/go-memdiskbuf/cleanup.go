package memdiskbuf

import (
	"os"
	"sync"
)

// Loop over any open temp files and clean them up, useful for a signal catcher
// cleanup.
func Cleanup() {
	for file, fh := range tmpFile {
		fh.Close()
		os.Remove(file)
	}
}

var (
	// Toggle debug
	Debug        = false
	tmpFile      = make(map[string]*os.File)
	tmpFileMutex sync.Mutex
)

// Create an is-used mark
func unuse(f string) {
	tmpFileMutex.Lock()
	defer tmpFileMutex.Unlock()
	delete(tmpFile, f)
}

// Create an is-used mark
func use(f string, fh *os.File) {
	tmpFileMutex.Lock()
	defer tmpFileMutex.Unlock()
	tmpFile[f] = fh
}
