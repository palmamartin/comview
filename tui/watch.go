package tui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"git.sr.ht/~rockorager/vaxis"

	"github.com/rockorager/comview/diff"
	"github.com/rockorager/comview/internal/terminal"
)

const defaultWatchInterval = 750 * time.Millisecond

type watchUpdateEvent struct {
	Rows    []diff.Row
	Message string
}

// RunWatch starts comview in watch mode. The command is rerun periodically and
// the displayed diff is refreshed whenever the command output changes.
func RunWatch(command []string) error {
	if len(command) == 0 {
		command = []string{"git", "diff"}
	}

	app, viewer, err := newDiffApp(nil)
	if err != nil {
		return err
	}
	viewer.emptyMessage = "No changes."
	viewer.emptyHint = fmt.Sprintf("Watching: %s", strings.Join(command, " "))
	viewer.setStatusMessage(fmt.Sprintf("Watching: %s", strings.Join(command, " ")))

	ctx, cancel := context.WithCancel(context.Background())
	app.OnClose(cancel)
	go watchCommand(ctx, app.Vaxis(), command, defaultWatchInterval)

	return app.Run()
}

func watchCommand(ctx context.Context, vx *vaxis.Vaxis, command []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastHash [sha256.Size]byte
	haveHash := false
	for {
		postCommandOutput(ctx, vx, command, &lastHash, &haveHash)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func postCommandOutput(ctx context.Context, vx *vaxis.Vaxis, command []string, lastHash *[sha256.Size]byte, haveHash *bool) {
	output, err := runWatchCommand(ctx, command)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		message := fmt.Sprintf("Watch command failed: %v", err)
		hash := sha256.Sum256([]byte("error:" + message))
		if *haveHash && hash == *lastHash {
			return
		}
		*lastHash = hash
		*haveHash = true
		vx.PostEvent(watchUpdateEvent{
			Message: message,
		})
		return
	}

	hash := sha256.Sum256([]byte("output:" + output))
	if *haveHash && hash == *lastHash {
		return
	}
	*lastHash = hash
	*haveHash = true

	rows, err := rowsForInput(output)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		vx.PostEvent(watchUpdateEvent{
			Message: fmt.Sprintf("Could not parse diff: %v", err),
		})
		return
	}

	message := fmt.Sprintf("Updated %s", time.Now().Format("15:04:05"))
	if len(rows) == 0 {
		message = fmt.Sprintf("No changes %s", time.Now().Format("15:04:05"))
	}
	vx.PostEvent(watchUpdateEvent{
		Rows:    rows,
		Message: message,
	})
}

func runWatchCommand(ctx context.Context, command []string) (string, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err == nil {
		return terminal.PrintableANSIOutput(bytes.NewReader(output)), nil
	}

	message := strings.TrimSpace(terminal.PrintableANSIOutput(&stderr))
	if message == "" {
		message = err.Error()
	}
	return "", errors.New(message)
}
