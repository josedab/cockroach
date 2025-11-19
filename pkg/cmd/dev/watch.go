// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

const (
	watchFlag       = "watch"
	watchDebounceMs = 500 // Debounce time in milliseconds
)

// watchState maintains the state for watch mode
type watchState struct {
	watcher      *fsnotify.Watcher
	debounceTime time.Duration
	lastRun      time.Time
	mutex        sync.Mutex
	stopChan     chan struct{}
}

// newWatchState creates a new watch state
func newWatchState() (*watchState, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	return &watchState{
		watcher:      watcher,
		debounceTime: watchDebounceMs * time.Millisecond,
		stopChan:     make(chan struct{}),
	}, nil
}

// addWatchFlag adds the --watch flag to a command
func addWatchFlag(cmd *cobra.Command) {
	cmd.Flags().Bool(watchFlag, false, "watch for file changes and re-run tests automatically")
}

// watchAndRun sets up file watching and runs the provided function on changes
func (d *dev) watchAndRun(
	ctx context.Context, pkgs []string, runFunc func() error,
) error {
	ws, err := newWatchState()
	if err != nil {
		return err
	}
	defer ws.watcher.Close()

	workspace, err := d.getWorkspace(ctx)
	if err != nil {
		return err
	}

	// Determine directories to watch based on packages
	dirsToWatch, err := d.getDirectoriesToWatch(ctx, workspace, pkgs)
	if err != nil {
		return err
	}

	if len(dirsToWatch) == 0 {
		return fmt.Errorf("no directories to watch")
	}

	// Add directories to watcher
	for _, dir := range dirsToWatch {
		if err := ws.watcher.Add(dir); err != nil {
			log.Printf("Warning: could not watch %s: %v", dir, err)
			continue
		}
		d.log.Printf("Watching: %s", dir)
	}

	log.Printf("Watch mode enabled. Watching %d directories. Press Ctrl+C to stop.", len(dirsToWatch))

	// Run initial test
	log.Println("Running initial test...")
	if err := runFunc(); err != nil {
		log.Printf("Initial test failed: %v", err)
	}

	// Start watching for changes
	var pendingRun bool
	var debounceTimer *time.Timer

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ws.stopChan:
			return nil
		case event, ok := <-ws.watcher.Events:
			if !ok {
				return nil
			}

			// Only react to relevant file changes
			if !isRelevantChange(event) {
				continue
			}

			d.log.Printf("File changed: %s (%s)", event.Name, event.Op)

			// Debounce multiple rapid changes
			ws.mutex.Lock()
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			pendingRun = true
			debounceTimer = time.AfterFunc(ws.debounceTime, func() {
				ws.mutex.Lock()
				if pendingRun {
					pendingRun = false
					ws.mutex.Unlock()

					log.Printf("\n--- Change detected in %s ---", filepath.Base(event.Name))
					if err := runFunc(); err != nil {
						log.Printf("Test failed: %v", err)
					}
					log.Println("Watching for changes...")
				} else {
					ws.mutex.Unlock()
				}
			})
			ws.mutex.Unlock()

		case err, ok := <-ws.watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

// getDirectoriesToWatch determines which directories should be watched
// based on the package specifications
func (d *dev) getDirectoriesToWatch(
	ctx context.Context, workspace string, pkgs []string,
) ([]string, error) {
	var dirs []string
	seen := make(map[string]bool)

	for _, pkg := range pkgs {
		pkg = strings.TrimPrefix(pkg, "//")
		pkg = strings.TrimPrefix(pkg, "./")
		pkg = strings.TrimSuffix(pkg, "/...")
		pkg = strings.TrimSuffix(pkg, ":all")

		// Remove target name if present
		if idx := strings.LastIndex(pkg, ":"); idx > 0 {
			pkg = pkg[:idx]
		}

		dir := filepath.Join(workspace, pkg)
		if seen[dir] {
			continue
		}
		seen[dir] = true

		// Check if directory exists
		exists, err := d.os.IsDir(dir)
		if err != nil || !exists {
			continue
		}

		dirs = append(dirs, dir)

		// Also watch testdata subdirectory if it exists
		testdataDir := filepath.Join(dir, "testdata")
		if exists, _ := d.os.IsDir(testdataDir); exists && !seen[testdataDir] {
			dirs = append(dirs, testdataDir)
			seen[testdataDir] = true
		}
	}

	return dirs, nil
}

// isRelevantChange checks if a file system event is relevant for re-running tests
func isRelevantChange(event fsnotify.Event) bool {
	// Only react to write and create events
	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return false
	}

	name := event.Name

	// Skip hidden files and directories
	base := filepath.Base(name)
	if strings.HasPrefix(base, ".") {
		return false
	}

	// React to Go files
	if strings.HasSuffix(name, ".go") {
		return true
	}

	// React to testdata files
	if strings.Contains(name, "/testdata/") {
		return true
	}

	return false
}
