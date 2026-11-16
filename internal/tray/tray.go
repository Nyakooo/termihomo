// Package tray presents a read-only summary from the terminal UI.
package tray

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"time"
)

const workerArg = "--tray-worker"

// Summary holds the proxy state and the localized detail lines shown in the tray.
type Summary struct {
	Active bool
	Lines  [4]string
}

type update struct {
	shown bool
	info  Summary
}

type Controller struct {
	ctx     context.Context
	updates chan update
	closed  chan struct{}
	last    update
}

func New(ctx context.Context) *Controller {
	if !supported() {
		return nil
	}
	return &Controller{ctx: ctx}
}

// Set is called by the UI loop. Only the latest pending summary is needed.
func (c *Controller) Set(shown bool, info Summary) {
	if c == nil {
		return
	}
	next := update{shown: shown, info: info}
	if next == c.last {
		return
	}
	c.last = next
	if c.updates == nil {
		c.updates, c.closed = make(chan update, 1), make(chan struct{})
		go c.run(c.ctx)
	}
	select {
	case <-c.updates:
	default:
	}
	c.updates <- next
}

// Wait briefly for the worker to remove its icon after the UI is closed.
func (c *Controller) Wait() {
	if c == nil || c.closed == nil {
		return
	}
	select {
	case <-c.closed:
	case <-time.After(time.Second):
	}
}

func (c *Controller) run(ctx context.Context) {
	var pipe io.WriteCloser
	var encoder *json.Encoder
	var done <-chan struct{}
	stop := func() {
		if pipe != nil {
			pipe.Close()
			pipe, encoder = nil, nil
		}
	}
	defer func() {
		stop()
		if done != nil {
			<-done
		}
		close(c.closed)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			stop()
			done = nil
		case next := <-c.updates:
			if !next.shown {
				stop()
				continue
			}
			if encoder == nil {
				if done != nil {
					select {
					case <-done:
					case <-ctx.Done():
						return
					}
				}
				var err error
				pipe, done, err = startWorker()
				if err != nil {
					continue
				}
				encoder = json.NewEncoder(pipe)
			}
			if err := encoder.Encode(next.info); err != nil {
				stop()
			}
		}
	}
}

func startWorker() (io.WriteCloser, <-chan struct{}, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	command, cleanup, err := workerCommand(executable)
	if err != nil {
		return nil, nil, err
	}
	command.Env = append(command.Environ(), "GH_TOKEN=", "GITHUB_TOKEN=")
	pipe, err := command.StdinPipe()
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := command.Start(); err != nil {
		pipe.Close()
		cleanup()
		return nil, nil, err
	}
	done := make(chan struct{})
	go func() {
		_ = command.Wait()
		cleanup()
		close(done)
	}()
	return pipe, done, nil
}

func directWorker(executable string) *exec.Cmd {
	return exec.Command(executable, workerArg)
}
