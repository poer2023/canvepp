//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework PDFKit -framework Foundation
#include <stdlib.h>
char *canveppPDFText(const char *path, int limit, int *status);
*/
import "C"

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
	"unsafe"
)

func extractPDFText(path string) (string, error) {
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var status C.int
	result := C.canveppPDFText(p, maxDocumentTextBytes, &status)
	if result != nil {
		defer C.free(unsafe.Pointer(result))
		return C.GoString(result), nil
	}
	switch status {
	case 2:
		return "", fmt.Errorf("PDF 已加密或限制文字提取，请提供可读取的副本")
	case 3:
		return "", fmt.Errorf("PDF 超过 200 页或文字超过 128 KB，请拆分后添加")
	case 4:
		return "", fmt.Errorf("PDF 未读到文字，请将扫描内容作为图片添加")
	default:
		return "", fmt.Errorf("无法读取此 PDF，请检查文件是否完整")
	}
}

type documentTextBuffer struct{ bytes.Buffer }

func (b *documentTextBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxDocumentTextBytes {
		return 0, fmt.Errorf("文档文字超过 128 KB，请拆分后添加")
	}
	return b.Buffer.Write(p)
}

func extractRichDocumentText(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// System parser only reads this staged file. No shell or auxiliary resources.
	cmd := exec.CommandContext(ctx, "/usr/bin/textutil", "-convert", "txt", "-stdout", "-encoding", "UTF-8", "-noload", "-nostore", "--", path)
	var out documentTextBuffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("文档解析失败或文字过长，请转成 PDF / 文本后添加")
	}
	return out.String(), nil
}
