package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

func (s *Store) generationNoticePreview(assets []Asset, dir string, strip bool) (path string, err error) {
	cache := ImageCache{store: s}
	var images []image.Image
	for _, asset := range assets {
		im, _, err := cache.decode(imageRequest{Asset: asset, Size: 640, Key: imageKey(asset, 640)})
		if err == nil && !im.Bounds().Empty() {
			images = append(images, im)
		}
	}
	if len(images) == 0 {
		return "", nil
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "generation-*.jpg")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			os.Remove(f.Name())
		}
	}()
	err = jpeg.Encode(f, generationNoticeMosaic(images, strip), &jpeg.Options{Quality: 86})
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	return filepath.Abs(f.Name())
}

func generationNoticeMosaic(images []image.Image, strip bool) image.Image {
	width, height := 272, 184
	if strip {
		width, height = 680, 108
	}
	im := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(im, im.Bounds(), image.NewUniform(color.Gray{Y: 115}), image.Point{}, draw.Src)
	gap := 4
	if len(images) == 1 {
		drawNoticeTile(im, im.Bounds(), images[0])
		return im
	}
	if strip {
		for i, src := range images {
			left := i * (width + gap) / len(images)
			right := (i+1)*(width+gap)/len(images) - gap
			drawNoticeTile(im, image.Rect(left, 0, right, height), src)
		}
		return im
	}
	// The Mac attachment uses a lead image and two stacked thumbnails.
	split := width * 3 / 5
	drawNoticeTile(im, image.Rect(0, 0, split, height), images[0])
	if len(images) == 2 {
		drawNoticeTile(im, image.Rect(split+gap, 0, width, height), images[1])
	} else {
		drawNoticeTile(im, image.Rect(split+gap, 0, width, (height-gap)/2), images[1])
		drawNoticeTile(im, image.Rect(split+gap, (height+gap)/2, width, height), images[2])
	}
	return im
}

func drawNoticeTile(dst *image.RGBA, tile image.Rectangle, src image.Image) {
	r := src.Bounds()
	w, h := r.Dx(), r.Dy()
	if w*tile.Dy() > h*tile.Dx() {
		crop := max(1, h*tile.Dx()/tile.Dy())
		r.Min.X += (w - crop) / 2
		r.Max.X = r.Min.X + crop
	} else {
		crop := max(1, w*tile.Dy()/tile.Dx())
		r.Min.Y += (h - crop) / 2
		r.Max.Y = r.Min.Y + crop
	}
	draw.CatmullRom.Scale(dst, tile, src, r, draw.Over, nil)
}
