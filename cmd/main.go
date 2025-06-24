package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

const fileName = "lazy.data"
const metaNode = "node-meta"

var nodes = [2]string{"node1", "node2"}

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
		return &File{
			Name:    fileName,
			Size:    1024 * 1024,
			Mode:    0444,
			ModTime: time.Now(),
		}, nil
	}
	return nil, syscall.Errno(syscall.ENOENT)
}

func (Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Println("Reading the dir")
	return []fuse.Dirent{
		{Inode: 2, Name: fileName, Type: fuse.DT_File},
	}, nil
}

type File struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
}

func (File) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 2
	a.Mode = 0444 // read-only
	a.Size = 1024 * 1024
	a.Mtime = time.Now()
	return nil
}

func dummyData() []byte {
	data := make([]byte, 1024*1024)
	for i := 0; i < len(data)-10; i += 10 {
		data[i] = 'Q'
		data[i+1] = 'U'
		data[i+2] = 'O'
		data[i+3] = 'I'
		data[i+4] = 'C'
		data[i+5] = 'O'
		data[i+6] = 'U'
		data[i+7] = 'B'
		data[i+8] = 'E'
		data[i+9] = 'H'
	}

	return data
}

func (f *File) WriteMeta(ctx context.Context) error {
	log.Println("Writing metadata to file")
	meta, err := json.Marshal(File{
		Name:    f.Name,
		Size:    f.Size,
		Mode:    f.Mode,
		ModTime: f.ModTime,
	})

	if err != nil {
		log.Printf("Failed to marshal metadata: %v", err)
		return err
	}

	if err := os.WriteFile(metaNode+"/"+fileName+"-meta.json", meta, 0444); err != nil {
		log.Printf("Failed to write metadata to file: %v", err)
		return err
	}

	return nil
}

func compareMeta(localInfo os.FileInfo, distantPath string) (bool, error) {
	local := &File{
		Name:    localInfo.Name(),
		Size:    localInfo.Size(),
		Mode:    localInfo.Mode(),
		ModTime: localInfo.ModTime().Local(),
	}

	jsonFile, err := os.Open(distantPath)
	if err != nil {
		return false, err
	}

	jsonData := json.NewDecoder(jsonFile)
	var meta File
	if err := jsonData.Decode(&meta); err != nil {
		log.Printf("Failed to decode metadata: %v", err)
		return false, err
	}

	switch {
	case meta.Name != local.Name:
		log.Printf("Metadata mismatch: expected name %s, got %s", local.Name, meta.Name)
		return false, nil
	case meta.Size != local.Size:
		log.Printf("Metadata mismatch: expected size %d, got %d", local.Size, meta.Size)
		return false, nil
	case meta.Mode != local.Mode:
		log.Printf("Metadata mismatch: expected mode %o, got %o", local.Mode, meta.Mode)
		return false, nil
	case (meta.ModTime.Unix()-local.ModTime.Unix()) > 1 || (local.ModTime.Unix()-meta.ModTime.Unix()) > 1: // abs value
		log.Printf("Metadata mismatch: expected modtime %v, got %v", local.ModTime, meta.ModTime)
		return false, nil
	default:
		log.Println("Metadata matches")
		return true, nil
	}

}

func (f *File) ReadAll(ctx context.Context) ([]byte, error) {
	// TODO(vi): Cache file in another folder if already request by os.WriteFile(..)
	// TODO(vi): Read the real_files when content is local os.ReadFile(..)
	log.Println("Lazy fetch triggered: generating dummy data")
	files, err := os.ReadDir("./mnt")
	if err != nil {
		log.Printf("Failed to read local directory: %v", err)
	}

	dummyData := dummyData()

	for _, file := range files {
		// TODO : remove this log later
		log.Printf("File name : %s\n", file.Name())
		for _, node := range nodes {
			realFilePath := node + "/" + file.Name()
			localInfo, err := os.Stat(realFilePath)
			if err != nil {
				log.Printf("Failed to read info of %s", file.Name())
				continue
			}
			ok, err := compareMeta(localInfo, metaNode+"/"+file.Name()+"-meta.json")
			if os.IsNotExist(err) || !ok {
				if err := os.WriteFile(realFilePath, dummyData, localInfo.Mode()); err != nil {
					log.Printf("Failed to write dummy data to %s: %v", realFilePath, err)
					return nil, err
				}

				if err := f.WriteMeta(ctx); err != nil {
					log.Printf("Failed to write metadata for %s: %v", file.Name(), err)
					return nil, err
				}
			} else {
				log.Printf("File %s already exists and metadata matches, skipping generation", file.Name())
			}
		}
	}

	return dummyData, nil
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
