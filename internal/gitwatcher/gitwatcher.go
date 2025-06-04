package gitwatcher

import (
	"sync"
	"time"

	"gobox/internal/gitutil"
)

// GitWatcher polls for new git commits since a start time and emits them via a channel.
type GitWatcher struct {
	StartTime    time.Time
	PollInterval time.Duration

	mu         sync.Mutex
	lastHashes map[string]struct{}
	stopCh     chan struct{}
	commitsCh  chan string
	errorCh    chan error
}

// NewGitWatcher creates a new GitWatcher.
func NewGitWatcher(startTime time.Time, pollInterval time.Duration) *GitWatcher {
	return &GitWatcher{
		StartTime:    startTime,
		PollInterval: pollInterval,
		lastHashes:   make(map[string]struct{}),
		stopCh:       make(chan struct{}),
		commitsCh:    make(chan string, 10),
		errorCh:      make(chan error, 2),
	}
}

// Start begins polling for new commits in a background goroutine.
func (gw *GitWatcher) Start() {
	go func() {
		ticker := time.NewTicker(gw.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-gw.stopCh:
				return
			case <-ticker.C:
				commits, err := gitutil.GetCommitsSince(gw.StartTime)
				if err != nil {
					gw.errorCh <- err
					continue
				}
				gw.mu.Lock()
				for _, commit := range commits {
					hash := ""
					if len(commit) > 8 {
						hash = commit[:8]
					} else {
						hash = commit
					}
					if _, seen := gw.lastHashes[hash]; !seen {
						gw.commitsCh <- commit
						gw.lastHashes[hash] = struct{}{}
					}
				}
				gw.mu.Unlock()
			}
		}
	}()
}

// Stop stops the polling goroutine.
func (gw *GitWatcher) Stop() {
	close(gw.stopCh)
}

// Commits returns a channel of new commit messages.
func (gw *GitWatcher) Commits() <-chan string {
	return gw.commitsCh
}

// Errors returns a channel of errors encountered during polling.
func (gw *GitWatcher) Errors() <-chan error {
	return gw.errorCh
}
