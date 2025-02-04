package bufferpool

import (
	"bytes"
	"sync"
)

var Pool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(nil)
	},
}

// Get returns a *bytes.Buffer that is managed by a sync.Pool.
// The buffer has already been reset and ready for use.
func Get() *bytes.Buffer {
	buf := Pool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// Put adds the *bytes.Buffer back into the sync.Pool.
func Put(buf *bytes.Buffer) {
	Pool.Put(buf)
}

// ReadCloser is a wrapper around *bytes.Buffer that implements io.ReadCloser.
// When it is closed, it returns the buffer to the sync.Pool.
type ReadCloser struct {
	*bytes.Buffer
}

// Close returns the buffer to the sync.Pool.
func (rc *ReadCloser) Close() error {
	Put(rc.Buffer)
	rc.Buffer = nil
	return nil
}
