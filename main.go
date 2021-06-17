package main

import (
	"bytes"
	"embed"
	iofs "io/fs"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

var (
	//go:embed root/*
	RootFS embed.FS
)

func do(status *int, mnt string) {
	defer os.RemoveAll(mnt)
	opts := &fs.Options{}
	opts.Debug = os.Getenv("debug") == "1"

	var (
		serviceDir = filepath.Join(mnt, "service")
		staticDir  = filepath.Join(mnt, "static")
		err        error
		serviceFS  = &Root{}
		staticFS   = &Root{}

		serviceServer, staticServer *fuse.Server
		cmd                         *exec.Cmd
		wd                          string

		// Go signal notification works by sending `os.Signal`
		// values on a channel. We'll create a channel to
		// receive these notifications (we'll also make one to
		// notify us when the program can exit).
		done     = make(chan int, 1)
		sigs     = make(chan os.Signal, 1)
		mainData []byte

		umountServicePort string
		umountService     = func(force bool) {
			if force {
				umount(serviceDir, nil)
			} else {
				umount(serviceDir, serviceServer)
			}
		}

		umountStaticPort string
		umountStatic     = func(force bool) {
			if force {
				umount(staticDir, nil)
			} else {
				umount(staticDir, staticServer)
			}
		}

		startChildPort string
		exe            string
	)

	if err = os.MkdirAll(serviceDir, 0o770); err != nil {
		log.Fatalf("Create directory %q fail: %v\n", serviceDir, err)
	}

	if err = os.MkdirAll(staticDir, 0o770); err != nil {
		log.Fatalf("Create directory %q fail: %v\n", staticDir, err)
	}

	if wd, err = os.Getwd(); err != nil {
		log.Fatalf("Read work directory fail: %v\n", err)
	}

	if exe, err = filepath.Abs(os.Args[0]); err != nil {
		log.Fatalf("Detect exe path fail: %v\n", err)
	}

	if mainData, err = RootFS.ReadFile("root/main.sh"); err != nil {
		log.Fatalf("Get main.sh data fail: %v\n", err)
	}
	mainData = append([]byte(script), mainData...)

	// ---- service FS ----
	if serviceFS.FS, err = iofs.Sub(RootFS, "root/service"); err != nil {
		log.Fatalf("Get subdirectory 'root/service' fail: %v\n", err)
	}

	log.Printf("Root directory mounted into %q.", mnt)
	log.Printf("press CTRL+C or send SIGINT or SIGTERM signals to stop server.")

	if serviceServer, err = fs.Mount(serviceDir, serviceFS, opts); err != nil {
		log.Fatalf("Mount service server fail: %v\n", err)
	}

	defer umountService(false)

	// ---- static FS ----
	if staticFS.FS, err = iofs.Sub(RootFS, "root/static"); err != nil {
		log.Fatalf("Get subdirectory 'root/static' fail: %v\n", err)
	}

	if staticServer, err = fs.Mount(staticDir, staticFS, opts); err != nil {
		log.Fatalf("Mount static server fail: %v\n", err)
	}

	defer umountStatic(false)

	// ---- server to umount service fs from root/main.sh ----

	umountServicePort = umountSignal(func() {
		umountService(true)
	})

	umountStaticPort = umountSignal(func() {
		umountStatic(true)
	})

	startChildPort = startChildServer()
	log.Println("start child server port:", startChildPort)

	cmd = exec.Command("sh", append([]string{"-s"}, os.Args[1:]...)...)
	cmd.Stdin = bytes.NewBuffer(mainData)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.Dir = mnt
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(cmd.Env,
		"cwd="+wd,
		"service_dir="+serviceDir,
		"static_dir="+staticDir,
		"umnt_service_port="+umountServicePort,
		"umnt_static_port="+umountStaticPort)

	cmds.onAdd = func(cmd *Cmd) {
		cmd.Env = append(cmd.Env,
			"start_child_port="+startChildPort,
			"exe="+exe,
		)
	}

	if err = cmds.Start(&Cmd{Cmd: cmd}); err != nil {
		log.Fatalf("Run `main.sh` fail: %v\n", err)
	}

	go func() {
		defer func() {
			if cmds.Count == 1 {
				done <- cmd.ProcessState.ExitCode()
			} else {
				close(done)
			}
		}()
		cmds.wg.Wait()
	}()

	// `signal.Notify` registers the given channel to
	// receive notifications of the specified signals.
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// This goroutine executes a blocking receive for
	// signals. When it gets one it'll print it out
	// and then notify the program that it can finish.
	go func() {
		sig := <-sigs
		log.Println()
		log.Println("received signal:", sig)
		for _, cmd := range cmds.cmds {
			if !cmd.Done {
				cmd.Process.Signal(sig)
			}
		}
	}()

	// The program will wait here until it gets the
	// expected signal (as indicated by the goroutine
	// above sending a value on `done`) and then exit.
	log.Println("awaiting signal")
	*status = <-done
}

func main() {
	if len(os.Args) > 2 {
		switch os.Args[1] {
		case "!cmd":
			initChildClient(os.Args[2], false, os.Args[3:]...)
		case "!script":
			initChildClient(os.Args[2], true, os.Args[3:]...)
		}
		return
	}

	var (
		mnt, err = ioutil.TempDir("", filepath.Base(os.Args[0]))
		status   int
	)

	if err != nil {
		log.Fatalf("Create temp dir fail: %v\n", err)
	}

	do(&status, mnt)
	log.Printf("Exit status: %d.", status)
	os.Exit(status)
}
