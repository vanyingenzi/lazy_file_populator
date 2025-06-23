package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

const fileName = "lazy.data"

type FS struct{}

func (FS) Root() (fs.Node, error) {
	return Dir{}, nil
}

type Dir struct{}

func (Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 1
	a.Mode = os.ModeDir | 0555
	return nil
}

func (Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	log.Println("Looking up the dir")
	if name == fileName {
		return File{}, nil
	}
	return nil, syscall.Errno(syscall.ENOENT)
}

func (Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Println("Reading the dir")
	// TODO(vi): Cache file in another folder if already request by os.WriteFile(..)
	// TODO(vi): Read the real_files when content is local os.ReadFile(..)
	return []fuse.Dirent{
		{Inode: 2, Name: fileName, Type: fuse.DT_File},
	}, nil
}

type File struct{}

func (File) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 2
	a.Mode = 0444 // read-only
	a.Size = 1024 * 1024
	a.Mtime = time.Now()
	return nil
}

func (File) ReadAll(ctx context.Context) ([]byte, error) {
	log.Println("Lazy fetch triggered: generating dummy data")
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = 'A'
	}
	return data, nil
}

func main() {
	mountpoint := "./mnt"
	log.Println("Mounting lazyfs at", mountpoint)

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName("lazyfs"),
		fuse.Subtype("lazyfetch"),
	)
	if err != nil {
		log.Fatalf("Mount failed: %v", err)
	}
	defer c.Close()

	// Handle Ctrl+C for graceful unmount
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Println("Received signal, unmounting...")
		if err := fuse.Unmount(mountpoint); err != nil {
			log.Printf("Failed to unmount: %v", err)
		}
		os.Exit(0)
	}()

	log.Println("Filesystem is now available")
	if err := fs.Serve(c, FS{}); err != nil {
		log.Fatalf("Serve failed: %v", err)
	}
}
