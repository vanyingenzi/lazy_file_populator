package main

import (
	"context"
	"log"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type FS struct{}

func (FS) Root() (fs.Node, error) {
	return File{}, nil
}

type File struct{}

func (File) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 1
	a.Mode = 0444 // read-only
	a.Size = 1024 * 1024 // pretend it's a 1MB file
	a.Mtime = time.Now()
	return nil
}

func (File) ReadAll(ctx context.Context) ([]byte, error) {
	log.Println("Lazy fetch triggered: generating dummy data")
	data := make([]byte, 1024*1024) // 1MB of dummy data
	for i := range data {
		data[i] = 'A'
	}
	return data, nil
}

func main() {
	mountpoint := "./mnt"
	log.Println("Starting ...")

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName("lazyfs"),
		fuse.Subtype("lazyfetch"),
		fuse.LocalVolume(),
		fuse.VolumeName("LazySyncthingFS"),
	)
	if err != nil {
		log.Fatalf("Mount failed: %v", err)
	}
	defer fuse.Unmount(mountpoint)
	defer c.Close()

	log.Printf("Mounted at %s", mountpoint)

	err = fs.Serve(c, FS{})
	if err != nil {
		log.Fatalf("Serve failed: %v", err)
	}

	<-c.Ready
	if err := c.MountError; err != nil {
		log.Fatalf("Mount error: %v", err)
	}
}
