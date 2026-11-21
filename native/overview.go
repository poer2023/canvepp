package main

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

const allCanvases = "全部画布"
const allPictures = "全部图片"
const overviewGap float32 = 4
const pictureInset float32 = 12

var pictureSortOptions = []string{"最近添加", "尺寸从大到小", "名称 A–Z"}
var pictureLayoutOptions = []string{"齐边拼排", "时间线"}

const pictureTimeline = 1

type pictureTile struct {
	entry    libraryEntry
	x, width float32
}

type pictureRow struct {
	y, height float32
	tiles     []pictureTile
}

type overviewState struct {
	title       string
	only        map[string]bool
	stamp       string
	rows        []pictureRow
	height      float32
	count       int
	query       string
	order       int
	search      searchControl
	selected    string
	imageScroll ui.ScrollState
	layout      int
	timeline    ui.ListState
	entries     []libraryEntry
}

func (a *desktop) pictureOverview() *overviewState {
	if a.left == generatedPictures {
		return &a.noticeOverview
	}
	return &a.overview
}

func (a *desktop) layoutOverview(width float32) {
	o := a.pictureOverview()
	var stamp strings.Builder
	order := o.order
	if o.layout == pictureTimeline {
		order = 0
	}
	fmt.Fprintf(&stamp, "%f/%d/%d/%q/%d", width, o.layout, order, o.query, a.runRevision)
	for _, id := range a.order {
		e := a.editors[id]
		fmt.Fprintf(&stamp, "/%s:%d:%d:%d:%s:%q", id, e.Project.Revision, len(e.Assets), len(e.Trash), e.Project.DeletedAt, e.Project.Name)
	}
	if o.stamp == stamp.String() {
		return
	}
	o.stamp = stamp.String()
	filter := parseLibraryQuery(o.query)
	var images []libraryEntry
	selectedVisible := false
	for _, id := range a.order {
		e := a.editors[id]
		if e.Project.DeletedAt != "" {
			continue
		}
		trashed := archivedPictureVersions(e)
		for _, img := range e.Assets {
			if trashed[img.ID] || o.only != nil && !o.only[img.ID] {
				continue
			}
			name := imageTitle(img)
			if len(filter) != 0 && !filter.matches(fmt.Sprintf("%s %s %s %s %d × %d", name, img.Name, e.Project.Name, a.assetPrompt(img), img.Width, img.Height)) {
				continue
			}
			date, _ := galleryDate(img.CreatedAt)
			key := id + "/" + img.ID
			selectedVisible = selectedVisible || o.selected == key
			images = append(images, libraryEntry{key: key, projectID: id, asset: &img, name: name, date: date})
		}
	}
	if !selectedVisible {
		o.selected = ""
	}
	var names *collate.Collator
	if order == 2 {
		names = collate.New(language.SimplifiedChinese, collate.IgnoreCase, collate.Numeric)
	}
	slices.SortFunc(images, func(x, y libraryEntry) int {
		var result int
		switch order {
		case 1:
			area := func(a *Asset) float64 { return float64(max(0, a.Width)) * float64(max(0, a.Height)) }
			result = cmp.Compare(area(y.asset), area(x.asset))
		case 2:
			result = names.CompareString(x.name, y.name)
		}
		if result != 0 {
			return result
		}
		if result = strings.Compare(y.date, x.date); result != 0 {
			return result
		}
		return strings.Compare(x.key, y.key)
	})
	o.count = len(images)
	o.entries = images
	if o.layout != pictureTimeline {
		o.rows, o.height = justifiedPictureRows(images, width)
	}
}

func archivedPictureVersions(e *Editor) map[string]bool {
	trashed := make(map[string]bool)
	for _, tr := range e.Trash {
		if e.Node(tr.Node.ID) != nil {
			continue
		}
		trashed[tr.Node.Asset] = true
		trashed[tr.Display] = true
		for _, id := range tr.Assets {
			trashed[id] = true
		}
	}
	// A version used by a live copy still belongs in the gallery.
	for _, n := range e.Project.Graph.Nodes {
		delete(trashed, n.Asset)
	}
	return trashed
}

func galleryPictureRatio(a *Asset) float32 {
	if a.Width <= 0 || a.Height <= 0 {
		return 1
	}
	return float32(a.Width) / float32(a.Height)
}

func smallGalleryPicture(a *Asset) bool {
	return a.Width > 0 && a.Height > 0 && max(a.Width, a.Height) <= 64
}

func justifiedPictureRows(images []libraryEntry, width float32) ([]pictureRow, float32) {
	var rows []pictureRow
	var y float32
	width = max(1, width)
	target := min(float32(154), max(float32(83), width/5.3))
	capHeight := target * 1.22
	hit := min(float32(36), width)

	tileWidth := func(image libraryEntry, height float32) float32 {
		w := galleryPictureRatio(image.asset) * height
		if smallGalleryPicture(image.asset) {
			w = min(w, float32(image.asset.Width*2+12))
		}
		return max(hit, w)
	}
	widthAt := func(items []libraryEntry, height float32) float32 {
		w := overviewGap * float32(len(items)-1)
		for _, image := range items {
			w += tileWidth(image, height)
		}
		return w
	}
	finish := func(items []libraryEntry, wide bool) {
		if len(items) == 0 {
			return
		}
		high := capHeight
		var tinyHeight float32
		for _, image := range items {
			if !smallGalleryPicture(image.asset) {
				tinyHeight = 0
				break
			}
			tinyHeight = max(tinyHeight, float32(image.asset.Height*2+12))
		}
		if tinyHeight > 0 {
			high = min(high, max(42, tinyHeight))
		}
		if wide {
			high = width / galleryPictureRatio(items[0].asset)
		} else {
			var low float32
			for range 18 {
				mid := (low + high) / 2
				if widthAt(items, mid) <= width {
					low = mid
				} else {
					high = mid
				}
			}
			high = low
		}
		row := pictureRow{y: y, height: max(38, high), tiles: make([]pictureTile, 0, len(items))}
		var x float32
		for _, image := range items {
			w := tileWidth(image, high)
			if wide {
				w = width
			}
			row.tiles = append(row.tiles, pictureTile{entry: image, x: x, width: w})
			x += w + overviewGap
		}
		rows = append(rows, row)
		y += row.height + overviewGap
	}

	start := 0
	for i := 0; i < len(images); i++ {
		image := images[i]
		if galleryPictureRatio(image.asset) >= 5 {
			pending := images[start:i]
			end := i + 1
			used := widthAt(pending, capHeight)
			// An adjacent photo can finish a partial row before a panorama,
			// while the panorama keeps the full width of its own row.
			for len(pending) > 0 && used < width && end < len(images) {
				next := images[end]
				count := len(pending) + end - i
				if galleryPictureRatio(next.asset) >= 5 || hit*float32(count)+overviewGap*float32(count-1) > width {
					break
				}
				used += overviewGap + tileWidth(next, capHeight)
				end++
			}
			if end > i+1 {
				pending = append(slices.Clone(pending), images[i+1:end]...)
			}
			finish(pending, false)
			finish(images[i:i+1], true)
			start, i = end, end-1
			continue
		}
		pending := images[start:i]
		used := widthAt(pending, target)
		candidate := used + overviewGap + tileWidth(image, target)
		minimum := hit*float32(i-start+1) + overviewGap*float32(i-start)
		if len(pending) > 0 && (minimum > width || candidate > width && width-used <= width*.17) {
			finish(pending, false)
			start = i
			candidate = tileWidth(image, target)
		}
		if candidate >= width {
			finish(images[start:i+1], false)
			start = i + 1
		}
	}
	finish(images[start:], false)
	return rows, max(0, y-overviewGap)
}

func (a *desktop) overviewPage(c *ui.Context) {
	o := a.pictureOverview()
	w, h := c.Size()
	width := max(1, w-a.sidebarWidth()-contentInset-2*pictureInset)
	title := allPictures
	if o.title != "" {
		title = o.title
	}
	ui.Column(c.Key(title)).Fill().Radius(10).Clip().Background(c.Theme().Background).Label(title + "画廊").Children(func() {
		pageHeader(c, title, "返回当前画布", func() { a.openWorkspace("画布") }, func() {
			searchWidth := min(float32(164), max(float32(76), width-182))
			if quietSearch(c, &o.query, &o.search, "搜索全部图片", "搜索图片", searchWidth) {
				o.imageScroll.Y = 0
				o.timeline.ScrollTo(0, ui.Start)
			}
			if o.layout != pictureTimeline && sortMenu(c, "全部图片排序", &o.order, pictureSortOptions) {
				o.imageScroll.Y = 0
			}
			icon := "board"
			if o.layout == pictureTimeline {
				icon = "calendar"
			}
			pickerMenu(c, "全部图片布局", icon, "布局", &o.layout, pictureLayoutOptions)
			a.layoutOverview(width)
			ui.Textf(c, "%d 张", o.count).FontSize(11).FontFeatures("tnum").TextColor(c.Theme().TextMuted)
		})
		results := ui.Column(c.Key("overview-results")).Grow(1).FillWidth().MinHeight(0).Clip()
		resultsMotion(c, results, resultsMotionKey{query: o.query, order: o.order, mode: o.layout, count: o.count}).Children(func() {
			if o.count == 0 {
				ui.Column(c).Fill().Center().Gap(8).Children(func() {
					scene, label := "library", "暂无图片"
					if strings.TrimSpace(o.query) != "" {
						scene, label = "search-empty", "没有匹配的图片"
					}
					emptyIcon(c, scene)
					ui.Text(c, label).FontSize(12).TextColor(c.Theme().TextMuted)
				})
				return
			}
			if o.layout == pictureTimeline {
				a.overviewTimeline(c, max(1, w-a.sidebarWidth()-contentInset-2*pageInset))
				return
			}
			scroll := &o.imageScroll
			thinScroll(c, ui.Scroll(c.Key("overview-pictures")).Label("全部图片滚动区"), scroll).
				FillWidth().Grow(1).MinHeight(0).Padding(8, pictureInset, 12, pictureInset).Children(func() {
				ui.Box(c).FillWidth().Height(o.height).Shrink(0).Children(func() {
					first := sort.Search(len(o.rows), func(i int) bool { return o.rows[i].y+o.rows[i].height >= scroll.Y-64 })
					for _, row := range o.rows[first:] {
						if row.y > scroll.Y+h+64 {
							break
						}
						for _, tile := range row.tiles {
							a.overviewPicture(c, tile, row, h)
						}
					}
				})
			})
		})
	})
}

func (a *desktop) overviewPicture(c *ui.Context, tile pictureTile, row pictureRow, windowHeight float32) {
	btn := a.overviewPictureButton(c, tile.entry).Absolute().Left(tile.x).Top(row.y).
		Size(tile.width, row.height).Background(c.Theme().Surface)
	a.overviewThumbnail(btn, *tile.entry.asset, windowHeight)
}

func (a *desktop) overviewPictureButton(c *ui.Context, item libraryEntry) ui.Element {
	asset := *item.asset
	o := a.pictureOverview()
	btn := ui.ButtonBase(c.Key(item.key)).Label("选择图片："+item.name).Selected(o.selected == item.key).
		Padding(0).Radius(4).Clip().Border(0, ui.Transparent).Background(ui.Transparent).Cursor(ui.CursorPointer)
	hint := fmt.Sprintf("%s · %d × %d · 空格查看大图", item.name, asset.Width, asset.Height)
	if o.selected == item.key {
		btn.DrawOver(func(p *ui.Painter, r ui.Rect) {
			p.Stroke(ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}, p.Theme().Text, 4, 2)
		})
		commandHint(c, btn, hint)
	} else {
		imageCommand(c, btn, hint)
	}
	return btn.OnClick(func() { o.selected = item.key; btn.Focus() }).
		HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputKeyDown || ev.Mods != 0 || ev.Key != ui.KeySpace || o.selected != item.key {
				return false
			}
			if !ev.Repeat && !a.previewOpen {
				a.preview(asset)
			}
			return true
		}).
		ContextMenu(func(menu *ui.Menu) { a.overviewPictureMenu(menu, item) })
}

func (a *desktop) overviewThumbnail(box ui.Element, asset Asset, windowHeight float32) {
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		fit := pictureThumbnailRect(r, &asset)
		p.Clip(fit, 4, func() { drawCheckers(p, fit, 10) })
		if a.images == nil || r.Y+r.H < workspaceHeaderHeight || r.Y > windowHeight {
			return
		}
		size := 512
		if max(fit.W, fit.H) > 500 {
			size = 1024
		}
		a.images.Request(asset, size)
		if b := a.images.Get(asset, size); b != nil {
			if smallGalleryPicture(&asset) {
				b = b.Pixelated()
			}
			p.Image(b, fit, ui.Contain)
		}
	})
}

func pictureThumbnailRect(box ui.Rect, image *Asset) ui.Rect {
	if image.Width <= 0 || image.Height <= 0 {
		return box
	}
	w, h := float32(image.Width), float32(image.Height)
	scale := min(box.W/w, box.H/h)
	if smallGalleryPicture(image) {
		scale = min(scale, 2)
	}
	w, h = w*scale, h*scale
	return ui.Rect{X: box.X + (box.W-w)/2, Y: box.Y + (box.H-h)/2, W: w, H: h}
}
