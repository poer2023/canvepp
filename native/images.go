package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

type cachedImage struct {
	Bitmap *ui.Bitmap
	Weight int64
	Used   uint64
	Err    string
}
type imageRequest struct {
	Asset   Asset
	Size    int
	Key     string
	Decoded image.Image
}

// Metadata and GPU bitmap ownership stay on the UI thread. Two workers perform
// all reads, validation, decoding and resampling before publishing immutable pixels.
type ImageCache struct {
	store   *Store
	entries map[string]*cachedImage
	pending map[string]bool
	queue   chan imageRequest
	stop    chan struct{}
	tick    uint64
	bytes   int64
	publish func(func())
}

func newImageCache(s *Store, publish func(func())) *ImageCache {
	c := &ImageCache{store: s, entries: map[string]*cachedImage{}, pending: map[string]bool{}, queue: make(chan imageRequest, 64), stop: make(chan struct{}), publish: publish}
	for i := 0; i < 2; i++ {
		go c.worker()
	}
	return c
}
func imageKey(a Asset, size int) string { return fmt.Sprintf("%s-%d", a.SHA, size) }
func (c *ImageCache) Request(a Asset, size int) {
	c.request(a, size, nil)
}
func (c *ImageCache) request(a Asset, size int, decoded image.Image) {
	k := imageKey(a, size)
	if v := c.entries[k]; v != nil {
		c.tick++
		v.Used = c.tick
		return
	}
	if c.pending[k] {
		return
	}
	// Only retain full decoded images when a worker can pick them up promptly.
	if len(c.queue) >= 2 {
		decoded = nil
	}
	select {
	case c.queue <- imageRequest{Asset: a, Size: size, Key: k, Decoded: decoded}:
		c.pending[k] = true
	default:
	}
}
func (c *ImageCache) Get(a Asset, size int) *ui.Bitmap {
	if v := c.entries[imageKey(a, size)]; v != nil {
		return v.Bitmap
	}
	return nil
}
func (c *ImageCache) worker() {
	for {
		select {
		case <-c.stop:
			return
		case req := <-c.queue:
			im, writeThumb, err := c.decode(req)
			var bitmap *ui.Bitmap
			var weight int64
			if err == nil {
				bitmap = ui.NewBitmap(im)
				weight = int64(im.Bounds().Dx() * im.Bounds().Dy() * 4)
			}
			c.publish(func() {
				delete(c.pending, req.Key)
				c.tick++
				v := &cachedImage{Bitmap: bitmap, Weight: weight, Used: c.tick}
				if err != nil {
					v.Err = err.Error()
				}
				c.entries[req.Key] = v
				c.bytes += weight
				c.evict(req.Key)
			})
			if writeThumb {
				var buf bytes.Buffer
				if err := png.Encode(&buf, im); err == nil {
					_ = atomicFile(filepath.Join(c.store.Dir, "thumbnails", req.Key+".png"), buf.Bytes())
				}
			}
		}
	}
}
func (c *ImageCache) evict(keep string) {
	for c.bytes > 64<<20 && len(c.entries) > 1 {
		key := ""
		used := uint64(math.MaxUint64)
		for k, v := range c.entries {
			if k != keep && v.Used < used {
				key = k
				used = v.Used
			}
		}
		if key == "" {
			return
		}
		c.bytes -= c.entries[key].Weight
		delete(c.entries, key)
	}
}
func (c *ImageCache) decode(r imageRequest) (image.Image, bool, error) {
	im := r.Decoded
	if im == nil {
		file, err := c.store.AssetPath(r.Asset)
		if err != nil {
			return nil, false, err
		}
		if r.Size > 0 {
			thumb := filepath.Join(c.store.Dir, "thumbnails", r.Key+".png")
			if b, err := os.ReadFile(thumb); err == nil {
				if cached, err := png.Decode(bytes.NewReader(b)); err == nil {
					return cached, false, nil
				}
			}
		}
		b, err := readBounded(file, maxStoredImageBytes)
		if err != nil {
			return nil, false, err
		}
		if digest(b) != r.Asset.SHA {
			return nil, false, fmt.Errorf("图片校验失败")
		}
		im, _, err = image.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, false, err
		}
	}
	w, h := im.Bounds().Dx(), im.Bounds().Dy()
	if r.Size > 0 && max(w, h) > r.Size {
		scale := float64(r.Size) / float64(max(w, h))
		dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))))
		draw.CatmullRom.Scale(dst, dst.Bounds(), im, im.Bounds(), draw.Src, nil)
		im = dst
	}
	return im, r.Size > 0, nil
}
