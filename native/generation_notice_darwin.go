//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#include <stdlib.h>
void canveppGenerationNotice(const char *id, const char *title, const char *body, const char *image);
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"unsafe"

	"github.com/egoist/mygo"
)

func generationNoticeScheme(string) string { return "" }

func deliverGenerationNotice(s *Store, n generationNotice) error {
	if !mygo.NotificationsSupported() {
		return fmt.Errorf("系统通知需要从 Canvepp.app 运行")
	}
	path, err := s.generationNoticePreview(n.images, os.TempDir(), false)
	if err != nil {
		log.Printf("生成通知缩略图：%v", err)
		path = ""
	}
	id, title, body, image := C.CString(generationNoticePrefix+n.id), C.CString(n.title), C.CString(n.body), C.CString(path)
	defer C.free(unsafe.Pointer(id))
	defer C.free(unsafe.Pointer(title))
	defer C.free(unsafe.Pointer(body))
	defer C.free(unsafe.Pointer(image))
	C.canveppGenerationNotice(id, title, body, image)
	return nil
}
