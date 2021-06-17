package main

import (
	"context"
	iofs "io/fs"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type Root struct {
	fs.Inode
	FS iofs.FS
}

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

func (this *File) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.f == nil {
		var err error
		if this.f, err = this.root.Open(this.path); err != nil {
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
	var (
		end = int(off) + len(dest)
		data []byte
	)
	var err error
	if data, err = iofs.ReadFile(this.root, this.path); err != nil {
		return nil, syscall.EIO
	}
	if end > len(data) {
		end = len(data)
	}
	return fuse.ReadResultData(data[off:end]), 0
}

func umount(pth string, srv *fuse.Server) {
	base := filepath.Base(pth)

	if _, err := os.Stat(pth); err == nil {
		defer os.RemoveAll(pth)
		log.Println("umounting", base)

		if srv != nil {
			err = srv.Unmount()
		} else {
			cmd := exec.Command("sudo", "umount", "-l", pth)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
		}

		if err != nil {
			log.Println("umount",base,"failed:", err)
		} else {
			log.Println(base,"umounted")
		}
	}
}

func umountSignal(umount func()) (port string) {
	// Listen for incoming connections on random port.
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatalf("Error listening: %s\n", err)
	}

	go func() {
		// Close the listener when the application closes.
		defer l.Close()
		for {
			// Listen for an incoming connection.
			_, err := l.Accept()
			if err != nil {
				log.Fatalf("Error listening: %s\n", err)
			}
			// umount busy device
			umount()
		}
	}()
	_, port, _ = net.SplitHostPort(l.Addr().String())
	return
}