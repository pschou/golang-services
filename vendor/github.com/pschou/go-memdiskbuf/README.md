# MemDiskBuffer

A hybrid buffer which preferrs to use the memory and then uses disk past a
defined threshold.  The value of using a buffer like this is to be able to
buffer a stream without knowing the final size.

Documentation:  https://pkg.go.dev/github.com/pschou/go-memdiskbuf
