package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	iofs "io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

//go:embed root/*
var FS embed.FS

type Root struct {
	fs.Inode
	FS iofs.FS
}

var _ = (fs.NodeOnAdder)((*Root)(nil))

func (this *Root) OnAdd(ctx context.Context) {
	iofs.WalkDir(this.FS, ".", func(path string, d iofs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		dir, base := filepath.Split(path)

		p := &this.Inode
		for _, component := range strings.Split(dir, "/") {
			if len(component) == 0 {
				continue
			}
			ch := p.GetChild(component)
			if ch == nil {
				ch = p.NewPersistentInode(ctx, &fs.Inode{},
					fs.StableAttr{Mode: fuse.S_IFDIR})
				p.AddChild(component, ch, true)
			}

			p = ch
		}
		ch := p.NewPersistentInode(ctx, &File{path: path, root: this.FS}, fs.StableAttr{})
		p.AddChild(base, ch, true)
		return nil
	})
}

// File is a file read from a zip archive.
type File struct {
	fs.Inode

	path string
	root iofs.FS

	mode os.FileMode
	mu   sync.Mutex
	f    iofs.File
	data []byte
}

// Getattr sets the minimum, which is the size. A more full-featured
// FS would also set timestamps and permissions.
func (this *File) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if this.f == nil {
		var err error
		if this.f, err = this.root.Open(this.path); err != nil {
			return syscall.EIO
		}
	}

	stat, _ := this.f.Stat()
	out.Mode = uint32(stat.Mode()) & 07777
	out.Nlink = 1
	out.Mtime = uint64(stat.ModTime().Unix())
	out.Atime = out.Mtime
	out.Ctime = out.Mtime
	out.Size = uint64(stat.Size())
	const bs = 512
	out.Blksize = bs
	out.Blocks = (out.Size + bs - 1) / bs
	return 0
}

// Open lazily unpacks zip data
func (this *File) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.f == nil {
		var err error
		if this.f, err = this.root.Open(this.path); err != nil {
			return nil, 0, syscall.EIO
		}
	}

	if this.data == nil {
		var err error
		if this.data, err = iofs.ReadFile(this.root, this.path); err != nil {
			return nil, 0, syscall.EIO
		}
	}

	// We don't return a filehandle since we don't really need
	// one.  The file content is immutable, so hint the kernel to
	// cache the data.
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

// Read simply returns the data that was already unpacked in the Open call
func (this *File) Read(ctx context.Context, f fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + len(dest)
	if end > len(this.data) {
		end = len(this.data)
	}
	return fuse.ReadResultData(this.data[off:end]), 0
}

var _ = (fs.NodeGetattrer)((*File)(nil))
var _ = (fs.NodeOnAdder)((*Root)(nil))

func main() {
	debug := flag.Bool("debug", false, "print debug data")
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  " + filepath.Base(os.Args[0]) + " [-debug] MOUNT_POINT")
	}
	opts := &fs.Options{}
	opts.Debug = *debug

	var (
		err  error
		root = &Root{}
	)

	if root.FS, err = iofs.Sub(FS, "root"); err != nil {
		log.Fatalf("Get subdirectory 'root' fail: %v\n", err)
	}
	log.Printf("Root directory mounted into %q.", flag.Arg(0))
	log.Printf("press CTRL+C or send SIGINT or SIGTERM signals to stop server.")

	server, err := fs.Mount(flag.Arg(0), root, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}

	// Go signal notification works by sending `os.Signal`
	// values on a channel. We'll create a channel to
	// receive these notifications (we'll also make one to
	// notify us when the program can exit).
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)

	// `signal.Notify` registers the given channel to
	// receive notifications of the specified signals.
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// This goroutine executes a blocking receive for
	// signals. When it gets one it'll print it out
	// and then notify the program that it can finish.
	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		done <- true
	}()

	// The program will wait here until it gets the
	// expected signal (as indicated by the goroutine
	// above sending a value on `done`) and then exit.
	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")
	if err = server.Unmount(); err != nil {
		fmt.Println("umount failed:", err)
	} else {
		fmt.Println("umounted")
	}
}
