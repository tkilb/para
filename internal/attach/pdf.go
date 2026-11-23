package attach

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/tkilb/para/internal/imgfmt"
	"github.com/tkilb/para/internal/loader"
	"github.com/tkilb/para/internal/store"
)

// pdfHeaderWindow is how far into a file the "%PDF-" header may start; the
// PDF specification allows some bytes before it.
const pdfHeaderWindow = 1024

// ErrNotPDF reports a file with a .pdf name that does not hold a PDF.
var ErrNotPDF = errors.New("the file is not a PDF")

// hasPDFHeader reports whether data starts a PDF.
func hasPDFHeader(data []byte) bool {
	return bytes.Contains(data[:min(len(data), pdfHeaderWindow)], []byte("%PDF-"))
}

// importPDF saves the PDF file at path unchanged into dir, named for the
// file. The file is only read, never opened or rendered.
func importPDF(s store.Store, dir, path string, now time.Time) (Result, error) {
	file, err := loader.ReadImage(path) // a plain, size-limited read
	if err != nil {
		return Result{}, err
	}
	if !hasPDFHeader(file.Data) {
		return Result{}, ErrNotPDF
	}
	return save(s, dir, fileStem(file.Name, store.PDFExt, now), store.PDFExt, file.Data)
}

// probePDF describes the PDF file at path for the import box, and reports
// false when it is not a readable file. It reads only the start of the file.
func probePDF(path string) (string, bool) {
	f, size, ok := openRegular(path)
	if !ok {
		return "", false
	}
	defer f.Close()
	if size > imgfmt.MaxBytes {
		return "PDF file, too large", true
	}
	head, err := io.ReadAll(io.LimitReader(f, pdfHeaderWindow))
	switch {
	case err != nil:
		return "", false
	case !hasPDFHeader(head):
		return "PDF file, not a PDF", true
	}
	return fmt.Sprintf(savedAsFormat, "PDF file", size), true
}
