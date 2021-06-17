package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

var cmds = &Commands{}

type Commands struct {
	cmds  []*Cmd
	wg    sync.WaitGroup
	onAdd func(cmd *Cmd)
	Count uint64
	mu sync.Mutex
}

func (this *Commands) Start(cmd *Cmd) (err error) {
	this.onAdd(cmd)
	if err = cmd.Start(); err != nil {
		return
	}
	this.mu.Lock()
	defer this.mu.Unlock()

	this.cmds = append(this.cmds, cmd)
	atomic.AddUint64(&this.Count, 1)
	this.wg.Add(1)
	go func() {
		defer func() {
			this.wg.Done()
			cmd.Done = true

			this.mu.Lock()
			defer this.mu.Unlock()

			var newCmds []*Cmd
			for _, cmd := range this.cmds {
				if !cmd.Done {
					newCmds = append(newCmds, cmd)
				}
			}
			this.cmds = newCmds
		}()
		cmd.Error = cmd.Wait()
	}()
	return nil
}

type CmdOpt struct {
	Name         string
	Args         []string
	Err, Out, In uintptr
	Script       string
}

type Cmd struct {
	*exec.Cmd
	Closers []io.Closer
	Error error
	Done bool
}

func (this *CmdOpt) Create() (cmd *Cmd) {
	if this.Script != "" {
		cmd = &Cmd{Cmd: exec.Command("sh", append([]string{"-s"}, os.Args[1:]...)...)}
		cmd.Stdin = bytes.NewBufferString(this.Script)
	} else {
		cmd = &Cmd{Cmd: exec.Command(this.Name, this.Args...)}
	}
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if this.Err > 0 && this.Err != os.Stderr.Fd() {
		cmd.Stderr = os.NewFile(this.Err, "stderr")
	}
	if this.Out > 0 && this.Out != os.Stdout.Fd() {
		cmd.Stdout = os.NewFile(this.Out, "stdout")
	}
	if this.In > 0 && this.In != os.Stdin.Fd() {
		cmd.Stdin = os.NewFile(this.In, "stdin")
	}
	return
}