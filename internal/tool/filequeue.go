package tool

import (
	"path/filepath"
	"sync"
)

var (
	fileMutationQueues   = map[string]chan struct{}{}
	fileMutationQueuesMu sync.Mutex
)

func withFileMutationQueue(filePath string, fn func() error) error {
	key := mutationQueueKey(filePath)

	fileMutationQueuesMu.Lock()
	queue, ok := fileMutationQueues[key]
	if !ok {
		queue = make(chan struct{}, 1)
		fileMutationQueues[key] = queue
	}
	fileMutationQueuesMu.Unlock()

	queue <- struct{}{}
	defer func() {
		<-queue
		fileMutationQueuesMu.Lock()
		if len(queue) == 0 {
			delete(fileMutationQueues, key)
		}
		fileMutationQueuesMu.Unlock()
	}()

	return fn()
}

func mutationQueueKey(filePath string) string {
	resolved := filepath.Clean(filePath)
	if real, err := filepath.EvalSymlinks(resolved); err == nil {
		return real
	}
	parent := filepath.Dir(resolved)
	if realParent, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(realParent, filepath.Base(resolved))
	}
	return resolved
}
