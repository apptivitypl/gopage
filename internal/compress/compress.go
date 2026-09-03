package compress

import (
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
)

const (
	MinCompressible = 512
	FastQuality     = 4
	EventType       = "text/event-stream"
	CodingBrotli    = "br"
	CodingGzip      = "gzip"
)

func Compressible(contentType string, size int) bool {
	if size < MinCompressible {
		return false
	}
	return compressibleType(contentType)
}

func Streamable(contentType string) bool {
	return compressibleType(contentType)
}

func compressibleType(contentType string) bool {
	if strings.HasPrefix(contentType, EventType) {
		return false
	}
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	switch {
	case strings.HasPrefix(contentType, "application/javascript"):
		return true
	case strings.HasPrefix(contentType, "application/json"):
		return true
	case strings.HasPrefix(contentType, "image/svg+xml"):
		return true
	default:
		return false
	}
}

func Brotli(sink io.Writer, source []byte) error {
	writer := brotli.NewWriterLevel(sink, brotli.BestCompression)
	_, err := writer.Write(source)
	return errors.Join(err, writer.Close())
}

var gzipWriters = sync.Pool{New: newGzipWriter}

func newGzipWriter() any {
	writer, _ := gzip.NewWriterLevel(io.Discard, gzip.BestCompression)
	return writer
}

func Gzip(sink io.Writer, source []byte) error {
	writer, _ := gzipWriters.Get().(*gzip.Writer)
	defer gzipWriters.Put(writer)
	writer.Reset(sink)
	_, err := writer.Write(source)
	return errors.Join(err, writer.Close())
}

var fastWriters = sync.Pool{New: newFastWriter}

func newFastWriter() any {
	return brotli.NewWriterLevel(io.Discard, FastQuality)
}

func Fast(sink io.Writer, source []byte) error {
	writer, _ := fastWriters.Get().(*brotli.Writer)
	defer fastWriters.Put(writer)
	writer.Reset(sink)
	_, err := writer.Write(source)
	return errors.Join(err, writer.Close())
}

type Stream struct {
	writer flushWriter
	put    func()
}

type flushWriter interface {
	io.WriteCloser
	Flush() error
}

func Open(coding string, sink io.Writer) *Stream {
	if coding == CodingGzip {
		writer, _ := gzipWriters.Get().(*gzip.Writer)
		writer.Reset(sink)
		return &Stream{writer: writer, put: func() { gzipWriters.Put(writer) }}
	}
	writer, _ := fastWriters.Get().(*brotli.Writer)
	writer.Reset(sink)
	return &Stream{writer: writer, put: func() { fastWriters.Put(writer) }}
}

func (s *Stream) Write(p []byte) (int, error) {
	return s.writer.Write(p)
}

func (s *Stream) Flush() error {
	return s.writer.Flush()
}

func (s *Stream) Close() error {
	err := s.writer.Close()
	s.put()
	return err
}
