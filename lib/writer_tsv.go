package lib

import (
	"bufio"
	"io"
	"strings"
)

type TsvWriter struct {
	bw *bufio.Writer
}

func NewTsvWriter(w io.Writer) *TsvWriter {
	return &TsvWriter{
		bw: bufio.NewWriter(w),
	}
}

func (w *TsvWriter) Write(record []string) error {
	_, err := w.bw.WriteString(strings.Join(record, "\t"))
	if err != nil {
		return err
	}
	return w.bw.WriteByte('\n')
}

func (w *TsvWriter) Flush() {
	w.bw.Flush()
}

func (w *TsvWriter) Error() error {
	return nil
}
