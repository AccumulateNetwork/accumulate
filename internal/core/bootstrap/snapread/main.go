package main
import (
    "fmt"
    "os"
    "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)
func main() {
    f, _ := os.Open(os.Args[1])
    defer f.Close()
    fi, _ := f.Stat()
    r, err := snapshot.Open(&fileSection{f, 0, fi.Size()})
    if err != nil { fmt.Println("err:", err); os.Exit(1) }
    h := r.Header
    fmt.Printf("RootHash: %x\n", h.RootHash)
}

// fileSection adapts an *os.File to ioutil.SectionReader.
type fileSection struct {
    *os.File
    base, size int64
}
func (s *fileSection) Size() int64 { return s.size }
