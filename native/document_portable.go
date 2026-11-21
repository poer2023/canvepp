//go:build !darwin || !cgo

package main

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-richdoc/richdoc"
	"github.com/go-rtf/rtf"
	"github.com/klippa-app/go-pdfium/requests"
)

var errDocumentTextLimit = errors.New("文档文字超过 128 KB，请拆分后添加")

func extractPDFText(path string) (string, error) {
	data, err := readBounded(path, maxAttachmentBytes)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := newLocalPDFPool(ctx)
	if err != nil {
		return "", errors.New("PDF 阅读器初始化失败")
	}
	defer pool.Close()
	pdf, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		return "", errors.New("PDF 阅读器初始化失败")
	}
	defer pdf.Close()
	document, err := pdf.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return "", errors.New("PDF 已加密或文件不完整，请提供可读取的副本")
	}
	security, err := pdf.FPDF_GetSecurityHandlerRevision(&requests.FPDF_GetSecurityHandlerRevision{Document: document.Document})
	if err != nil || security.SecurityHandlerRevision >= 0 {
		return "", errors.New("PDF 已加密或限制文字提取，请提供可读取的副本")
	}
	pages, err := pdf.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: document.Document})
	if err != nil || pages.PageCount < 1 {
		return "", errors.New("无法读取此 PDF，请检查文件是否完整")
	}
	if pages.PageCount > 200 {
		return "", errors.New("PDF 超过 200 页，请拆分后添加")
	}
	var text strings.Builder
	for page := range pages.PageCount {
		plain, err := pdf.GetPageText(&requests.GetPageText{Page: requests.Page{ByIndex: &requests.PageByIndex{Document: document.Document, Index: page}}})
		if err != nil {
			return "", errors.New("PDF 文字提取失败，请检查文件或拆分后添加")
		}
		if text.Len()+len(plain.Text)+1 > maxDocumentTextBytes {
			return "", errDocumentTextLimit
		}
		text.WriteString(plain.Text)
		text.WriteByte('\n')
	}
	return text.String(), nil
}

func extractRichDocumentText(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx":
		return extractArchiveDocument(path, "word/document.xml")
	case ".odt":
		return extractArchiveDocument(path, "content.xml")
	case ".rtf":
		data, err := readBounded(path, maxAttachmentBytes)
		if err != nil {
			return "", err
		}
		document, err := rtf.Parse(data)
		if err != nil {
			return "", errors.New("RTF 文件解析失败")
		}
		return richdoc.PlainText(document), nil
	default:
		return "", errors.New("请将旧版 Word 文档另存为 DOCX 或文本后添加")
	}
}

func extractArchiveDocument(path, member string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", errors.New("文档已加密或文件不完整")
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name != member {
			continue
		}
		if file.UncompressedSize64 > maxAttachmentBytes {
			return "", errors.New("文档内容超过大小限制，请拆分后添加")
		}
		stream, err := file.Open()
		if err != nil {
			return "", errors.New("无法读取文档正文")
		}
		defer stream.Close()
		return extractDocumentXML(stream, member == "word/document.xml")
	}
	return "", errors.New("文档缺少正文")
}

// Extract text from document bodies only. Archive relationships, scripts,
// embedded objects and external references are never followed.
func extractDocumentXML(r io.Reader, word bool) (string, error) {
	const (
		wordNS       = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
		wordStrictNS = "http://purl.oclc.org/ooxml/wordprocessingml/main"
		odtNS        = "urn:oasis:names:tc:opendocument:xmlns:text:1.0"
	)
	decoder := xml.NewDecoder(io.LimitReader(r, maxAttachmentBytes+1))
	var text strings.Builder
	paragraphs, runs := 0, 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return text.String(), nil
		}
		if err != nil {
			return "", errors.New("文档正文结构损坏")
		}
		switch element := token.(type) {
		case xml.StartElement:
			name := element.Name
			switch {
			case word && (name.Space == wordNS || name.Space == wordStrictNS):
				switch name.Local {
				case "p":
					paragraphs++
				case "t":
					runs++
				case "br", "cr":
					text.WriteByte('\n')
				case "tab":
					text.WriteByte('\t')
				}
			case !word && name.Space == odtNS:
				switch name.Local {
				case "p", "h":
					paragraphs++
				case "line-break":
					text.WriteByte('\n')
				case "tab":
					text.WriteByte('\t')
				case "s":
					text.WriteByte(' ')
				}
			}
		case xml.EndElement:
			name := element.Name
			if word && (name.Space == wordNS || name.Space == wordStrictNS) && name.Local == "t" {
				runs--
			}
			if (word && (name.Space == wordNS || name.Space == wordStrictNS) && name.Local == "p") ||
				(!word && name.Space == odtNS && (name.Local == "p" || name.Local == "h")) {
				paragraphs--
				text.WriteByte('\n')
			}
		case xml.CharData:
			if paragraphs > 0 && (!word || runs > 0) {
				if text.Len()+len(element) > maxDocumentTextBytes {
					return "", errDocumentTextLimit
				}
				text.Write(element)
			}
		}
		if text.Len() > maxDocumentTextBytes {
			return "", errDocumentTextLimit
		}
	}
}
