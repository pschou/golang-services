package memdiskbuf

import (
	"errors"
	"fmt"
	"io"
	"sort"
)

// A buffer for WriteAt calls.  The practical use for this module is to
// reconstruct an original file when only specific fragments are available at
// any given moment, such as from a semi-ordered UDP stream.  Note: A larger
// buffer size will prevent buffer overruns when UDP traffic has more sever
// unordering in packets.
//
// This WriterAtBuf
// assumes the payload being written is immutable, meaning the bytes written
// and then re-written will never change.
//
// An example of a UDP buffer for file transfers:
//
//   fh, _ := os.Create("my.dat")
//   wab := NewWriterAtBuf(fh, 20<<10)  // A generous 20k data ordering buffer
//   // Listen for UDP packets with offset positioning
//   wab.WriteAt(UDPdata, offset)
type WriterAtBuf struct {
	fh             io.WriterAt
	written, block int64

	// User provided function for in-memory inspection of the data before it is
	// written to disk.  A useful way to use this is for calling a Hash function
	// over the data so it doesn't have to be re-read from the disk.
	StreamFunc  func([]byte)
	streamFlush int

	buf   []byte
	bufSt int64
	inbuf []startStop
}

type startStop struct {
	start, stop int64
}

// Reset will reset the counters and reuse the underlying buffer for another
// WriterAt interface.
func (w *WriterAtBuf) Reset(fh io.WriterAt) {
	w.fh = fh
	w.written = 0
	w.StreamFunc, w.streamFlush = nil, 0
	w.bufSt = 0
	w.inbuf = make([]startStop, 3)
}

// NewWriterAtBuf creates a new WriterAtBuf for buffering WriteAt calls with a
// specified bufSize.  For optimal performance, the BufSize should be at least
// 8k and preferably around 32k.
func NewWriterAtBuf(fh io.WriterAt, bufSize int) *WriterAtBuf {
	return &WriterAtBuf{
		fh:    fh,
		buf:   make([]byte, bufSize),
		inbuf: make([]startStop, 3),
		block: 4 << 10,
	}
}

// NewWriterAtBufWithBlockSize creates a new WriterAtBuf for buffering WriteAt
// calls with a specified bufSize.  For optimal performance, the BufSize should
// be at least 8k and preferably around 32k.  The block size should be a 2^n
// multiple of 4k, like 4k, 8k, ... to ensure the best performance for sector
// size matching.
//
// The bufSize must be greater than twice the blockSize to ensure that the
// block can properly be blocked off and written to the destination.
func NewWriterAtBufWithBlockSize(fh io.WriterAt, bufSize, blockSize int) *WriterAtBuf {
	if bufSize < blockSize*2 {
		bufSize = blockSize * 2
	}
	return &WriterAtBuf{
		fh:    fh,
		buf:   make([]byte, bufSize),
		inbuf: make([]startStop, 3),
		block: int64(blockSize),
	}
}

// WriteAt writes len(b) bytes to the File starting at byte offset off. It
// returns the number of bytes written and an error, if any. WriteAt returns a
// non-nil error when n != len(b).
func (w *WriterAtBuf) WriteAt(p []byte, off int64) (n int, err error) {
	offEnd, bufEnd := off+int64(len(p)), w.bufSt+int64(len(w.buf))
	if off < w.bufSt { // We've moved past this point
		if offEnd <= w.bufSt { // no-op
			return len(p), nil
		}
		// We can pick up a bit of what's left for the buffer, it should not come
		// to this point if the data is not overlapping, but this method account
		// for some randomness
		n = copy(w.buf[w.bufSt-off:], p)
		add(&w.inbuf, w.bufSt, w.bufSt+int64(n))
		n += int(w.bufSt - off)
		err = w.shift()
		return
	}
	if offEnd > bufEnd {
		// Miss, trigger an error
		return 0, fmt.Errorf("WriteAt called outside buffer window %d-%d > [%d-%d], try increasing the buffer size.",
			off, offEnd, w.bufSt, bufEnd)
	}
	// Buffer hit!
	add(&w.inbuf, off, offEnd)
	n = copy(w.buf[off-w.bufSt:], p)
	err = w.shift()
	return
}

func (w *WriterAtBuf) shift() (err error) {
	var n int
	for chunkEnd := w.bufSt + w.block; w.inbuf[0].start == w.bufSt && w.inbuf[0].stop >= chunkEnd; chunkEnd = w.bufSt + w.block {
		// We have data!  Call the stream function in case we have one
		if w.StreamFunc != nil {
			w.StreamFunc(w.buf[w.streamFlush:w.block])
			w.streamFlush = 0
		}

		// Good times, go ahead and write our block
		if n, err = w.fh.WriteAt(w.buf[:w.block], w.inbuf[0].start); err != nil {
			return // Something bad happened
		} else if n != int(w.block) {
			return fmt.Errorf("Could not write %d, instead wrote %d, check the Block size used.", w.block, n)
		}
		w.bufSt, w.inbuf[0].start, w.written = chunkEnd, chunkEnd, chunkEnd+int64(n)
		for i, j := 0, int(w.block); j < len(w.buf); i, j = i+1, j+1 {
			w.buf[i] = w.buf[j]
		}
	}
	return
}

// Total amount written to disk
func (w *WriterAtBuf) Written() int64 {
	return w.written
}

// FlushAll - flushes all the buffer to disk, gaps and all.  Doesn't advance
// the buffer to prevent blocks getting out of sync.  An error will be returned
// if there was an error writing to the target file.
//
// Note: FlushAll does not call the StreamFunc as Flush() is the proper way to
// ensure all the segments are written correctly.  FlushAll is a sort of "give
// up" function when an error in writing happens and one wants to get the
// current state of a file, gaps and all.
func (w *WriterAtBuf) FlushAll() (err error) {
	var n int
	for _, inbuf := range w.inbuf {
		if inbuf.start >= w.bufSt {
			toWrite := inbuf.stop - inbuf.start
			if n, err = w.fh.WriteAt(w.buf[:toWrite], inbuf.start-w.bufSt); err != nil {
				return // Something bad happened
			} else if n != int(toWrite) {
				return fmt.Errorf("Could not write %d, instead wrote %d, check the Block size used.", w.block, n)
			}
		}
	}
	return nil
}

// Flush - flushes what is in the buffer to disk, but don't advance the buffer
// to prevent blocks getting out of sync.  An error will be returned if there
// are gaps in the buffer.
func (w *WriterAtBuf) Flush() (err error) {
	var n int
	if w.inbuf[0].start == w.bufSt {
		toWrite := w.inbuf[0].stop - w.inbuf[0].start

		// We have data!  Call the stream function in case we have one
		if w.StreamFunc != nil {
			w.StreamFunc(w.buf[w.streamFlush:toWrite])
			w.streamFlush = int(toWrite)
		}

		if n, err = w.fh.WriteAt(w.buf[:toWrite], w.inbuf[0].start); err != nil {
			return // Something bad happened
		} else if n != int(toWrite) {
			return fmt.Errorf("Could not write %d, instead wrote %d, check the Block size used.", w.block, n)
		}

		// Update counters
		w.written = w.bufSt + int64(n)
	}
	for _, b := range w.inbuf {
		if b.stop > w.written {
			return errors.New("Could not flush, missing one or more segments")
		}
	}
	return nil
}

// Flushable returns the flushable size of a file if a Flush is called.  An
// error is returned should there be any gaps.
func (w *WriterAtBuf) Flushable() (n int64, err error) {
	if w.inbuf[0].start == w.bufSt {
		n = w.bufSt + (w.inbuf[0].stop - w.inbuf[0].start)
	}
	for _, b := range w.inbuf {
		if b.stop > w.written {
			err = errors.New("Could not flush, missing one or more segments")
			break
		}
	}
	return
}

// Add will add a fragment to a start-stop list for defragmenting.
func add(set *[]startStop, start, stop int64) {
	recPos := -1 // Mark any open position for assignment
	seen := *set
	defer condense(*set) // Upon add finishing, make sure the resultant list is condensed
	for i := range seen {
		if seen[i].stop == 0 && recPos == -1 {
			recPos = i
		}
		if seen[i].stop >= start && stop >= seen[i].start { // Append to an existing block
			seen[i].start, seen[i].stop = min(seen[i].start, start), max(seen[i].stop, stop)
			return
		}
	}
	if recPos == -1 {
		seen = append(seen, startStop{start: start, stop: stop})
		*set = seen
	} else {
		seen[recPos].start, seen[recPos].stop = start, stop
	}
}

// Bring all the fragments together, defragment them, and build larger blocks
// with 0 being the first.
func condense(seen []startStop) {
	// Sort the values to ensure the smallest is at the front
	sort.Slice(seen, func(i, j int) bool {
		if seen[j].stop == 0 && seen[i].stop > 0 { // send zeros to the end
			return true
		} else if seen[i].stop == 0 && seen[j].stop > 0 { // send zeros to the end
			return false
		}
		return seen[i].start < seen[j].start
	})

	var i int
	var workDone bool
	for i < len(seen) {
		if seen[i].stop == 0 {
			i++
			continue
		}
		workDone = false
		for j := i + 1; j < len(seen); j++ {
			if seen[j].stop == 0 {
				continue
			}
			if seen[i].stop+1 >= seen[j].start && seen[j].stop+1 >= seen[i].start {
				seen[i].start, seen[i].stop, seen[j].start, seen[j].stop =
					min(seen[i].start, seen[j].start), max(seen[i].stop, seen[j].stop), 0, 0
				workDone = true
			}
		}
		if !workDone {
			i, workDone = i+1, false
		}
	}
}
