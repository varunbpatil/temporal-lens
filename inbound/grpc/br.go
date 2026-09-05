package grpc

import (
	"io"

	"connectrpc.com/connect"
	"github.com/andybalholm/brotli"
)

// Brotli is the IANA name for the Brotli compression algorithm.
const Brotli = "br"

// NewBrotliCompressor returns a brotli compressor.
func NewBrotliCompressor() connect.Compressor {
	return &brotliCompressor{w: brotli.NewWriter(nil)}
}

// NewBrotliDecompressor returns a brotli decompressor.
func NewBrotliDecompressor() connect.Decompressor {
	return &brotliDecompressor{r: brotli.NewReader(nil)}
}

type brotliCompressor struct {
	w *brotli.Writer
}

func (c *brotliCompressor) Write(p []byte) (int, error) {
	return c.w.Write(p)
}

func (c *brotliCompressor) Close() error {
	return c.w.Close()
}

func (c *brotliCompressor) Reset(w io.Writer) {
	c.w.Reset(w)
}

type brotliDecompressor struct {
	r *brotli.Reader
}

func (d *brotliDecompressor) Read(p []byte) (int, error) {
	return d.r.Read(p)
}

func (d *brotliDecompressor) Close() error {
	return nil
}

func (d *brotliDecompressor) Reset(r io.Reader) error {
	return d.r.Reset(r)
}
