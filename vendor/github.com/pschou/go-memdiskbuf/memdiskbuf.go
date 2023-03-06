package memdiskbuf

import (
	"errors"
	"io"
	"os"
)

type Buffer struct {
	// Counters
	i, n int64

	// Starting buffer size
	st []byte

	// Details about the file to be operated on
	ibuf      int
	buf       []byte
	isReading bool

	path string
	fh   *os.File
}

// When calling a new path, provide a file name which could potentially be used
// for a buffer once the minimum threshold has been met.  Any writes below the
// threshold will not touch the disk, while any writes over the threshold will.
//
// Please note that DiskBuf should be a power of 2 multiplied by 4KB, so a good
// value would be 16K.  This is because the sector size of a disk or NAND
// memory is usually 4k.
func NewBuffer(path string, MemBuf, DiskBuf int) *Buffer {
	return &Buffer{path: path, st: make([]byte, MemBuf), buf: make([]byte, DiskBuf)}
}

// Write to the Buffer, first filling up the memory buffer and then to the disk.
func (b *Buffer) Write(p []byte) (n int, err error) {
	if b.isReading {
		return 0, errors.New("Already in read mode")
	}

	if b.n < int64(len(b.st)-1) {
		n = copy(b.st[b.n:], p)
		b.n += int64(n)
		p = p[n:] // sub-slice the remaining writes
	}

	for len(p) > 0 { // as long as we have more to read
		c := copy(b.buf[b.ibuf:], p)
		b.n, n, b.ibuf = b.n+int64(c), n+c, b.ibuf+c
		p = p[c:]

		if b.ibuf > len(b.buf)/2 {
			if err = b.commit(len(b.buf) / 2); err != nil {
				return
			}
		}
	}
	return
}

// Write out the disk buffer
func (b *Buffer) commit(ct int) (err error) {
	if b.fh == nil {
		if b.fh, err = os.OpenFile(b.path, os.O_RDWR|os.O_CREATE, 0600); err != nil {
			use(b.path, b.fh)
			return
		}
	}
	toWrite, st := b.buf[:ct], b.n-int64(b.ibuf)-int64(len(b.st))
	var n int
	for len(toWrite) > 0 {
		if n, err = b.fh.WriteAt(toWrite, st); err != nil {
			return
		}
		toWrite, st = toWrite[n:], st+int64(n)
	}
	for i, j := 0, ct; j < b.ibuf; i, j = i+1, j+1 {
		b.buf[i] = b.buf[j] // shift bytes over n
	}
	b.ibuf -= ct
	return
}

// Until a Buffer is reset, the buffer can be Rewound to restart from the beginning.
func (b *Buffer) Rewind() {
	b.i = 0
}

// Clear out the buffer and switch to reading writing
func (b *Buffer) Reset() {
	if b.fh != nil {
		b.fh.Close()
		b.fh = nil
		unuse(b.path)
		os.Remove(b.path)
	}
	b.i, b.n, b.ibuf, b.isReading = 0, 0, 0, false
}

// Read reads from the buffer.  The first read will switch the buffer from
// writing mode to reading mode to prevent further writes.  One can use Reset()
// to clear out the buffer and return to writing mode.
func (b *Buffer) Read(p []byte) (n int, err error) {
	if !b.isReading {
		if b.ibuf > 0 {
			if err = b.commit(b.ibuf); err != nil {
				return
			}
		}
		b.isReading = true
	}
	if b.i < int64(len(b.st)) {
		if b.n < int64(len(b.st)) {
			n = copy(p, b.st[b.i:b.n])
		} else {
			n = copy(p, b.st[b.i:])
		}
		b.i = b.i + int64(n)
		if b.i == b.n {
			return n, io.EOF
		}
		p = p[n:] // Read the remaining from the disk
	}

	var c int
	for err == nil && len(p) > 0 {
		c, err = b.fh.ReadAt(p, b.i-int64(len(b.st)))
		n, b.i, p = n+c, b.i+int64(c), p[c:]
	}
	if b.i == b.n {
		return n, io.EOF
	}
	return
}

// ReadAt reads from the buffer.  The first read will switch the buffer from
// writing mode to reading mode to prevent further writes.  One can use Reset()
// to clear out the buffer and return to writing mode.
func (b *Buffer) ReadAt(p []byte, offset int64) (n int, err error) {
	if !b.isReading {
		if b.ibuf > 0 {
			if err = b.commit(b.ibuf); err != nil {
				return
			}
		}
		b.isReading = true
	}
	if offset < int64(len(b.st)) {
		if b.n < int64(len(b.st)) {
			n = copy(p, b.st[offset:b.n])
		} else {
			n = copy(p, b.st[offset:])
		}
		offset += int64(n)
		if offset == b.n {
			return n, io.EOF
		}
		p = p[n:] // Read the remaining from the disk
	}

	var c int
	for err == nil && len(p) > 0 {
		c, err = b.fh.ReadAt(p, offset-int64(len(b.st)))
		n, offset, p = n+c, b.i+int64(c), p[c:]
	}
	if offset == b.n {
		return n, io.EOF
	}
	return
}

// Return the length of the remaining used Buffer.
func (b Buffer) Len() int64 {
	return b.n - b.i
}

// Return the length of the Buffer.
func (b Buffer) Cap() int64 {
	return b.n
}
