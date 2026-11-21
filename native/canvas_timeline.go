package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

const (
	timelineUpdated = iota
	timelineOpened
	timelineCreated
	timelineName
)

var timelineSortOptions = []string{"最近更新", "最近打开", "最近创建", "名称 A–Z"}

type canvasTimeline struct {
	order  int
	query  string
	search searchControl
	list   ui.ListState
	visits map[string]time.Time
}

const canvasVisitPrefix = "canvas_opened:"

func (t *canvasTimeline) loadVisits(store *Store) error {
	rows, err := store.DB.Query("SELECT key, value FROM native_meta WHERE key GLOB ?", canvasVisitPrefix+"*")
	if err != nil {
		return err
	}
	defer rows.Close()
	t.visits = make(map[string]time.Time)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		if opened, err := time.Parse(time.RFC3339Nano, value); err == nil {
			t.visits[strings.TrimPrefix(key, canvasVisitPrefix)] = opened
		}
	}
	return rows.Err()
}

func (a *desktop) rememberCanvasOpen(e *Editor) {
	if e.Project.DeletedAt != "" {
		return
	}
	now := time.Now()
	if a.canvasTimeline.visits == nil {
		a.canvasTimeline.visits = make(map[string]time.Time)
	}
	a.canvasTimeline.visits[e.Project.ID] = now
	if a.store != nil {
		if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES(?,?)", canvasVisitPrefix+e.Project.ID, now.UTC().Format(time.RFC3339Nano)); err != nil {
			a.message("保存画布打开时间失败：" + err.Error())
		}
	}
}

type canvasTimelineCard struct {
	editor *Editor
	date   time.Time
}

type canvasTimelineRow struct {
	key   string
	title string
	cards []canvasTimelineCard
}

func (a *desktop) canvasTimelineCards() []canvasTimelineCard {
	t := &a.canvasTimeline
	query := parseLibraryQuery(t.query)
	cards := make([]canvasTimelineCard, 0, len(a.order))
	for _, id := range a.order {
		e := a.editors[id]
		p := &e.Project
		if p.DeletedAt != "" || !query.matches(p.Name) {
			continue
		}
		var date time.Time
		switch t.order {
		case timelineOpened:
			date = t.visits[id]
		case timelineCreated:
			date, _ = time.Parse(time.RFC3339Nano, p.CreatedAt)
		default:
			date, _ = time.Parse(time.RFC3339Nano, p.UpdatedAt)
		}
		cards = append(cards, canvasTimelineCard{editor: e, date: date})
	}
	if t.order == timelineName {
		names := collate.New(language.SimplifiedChinese, collate.IgnoreCase, collate.Numeric)
		slices.SortStableFunc(cards, func(x, y canvasTimelineCard) int {
			return names.CompareString(x.editor.Project.Name, y.editor.Project.Name)
		})
	} else {
		slices.SortStableFunc(cards, func(x, y canvasTimelineCard) int { return y.date.Compare(x.date) })
	}
	return cards
}

func canvasTimelineRows(cards []canvasTimelineCard, order, columns int, now time.Time) []canvasTimelineRow {
	var rows []canvasTimelineRow
	group := ""
	for _, card := range cards {
		title := timelineDateGroup(card.date, now)
		if order == timelineName {
			title = "按名称"
		}
		if group != title {
			rows = append(rows, canvasTimelineRow{key: "heading/" + title, title: title})
			group = title
		}
		last := &rows[len(rows)-1]
		if last.title != "" || len(last.cards) >= columns {
			rows = append(rows, canvasTimelineRow{key: "cards/" + card.editor.Project.ID})
			last = &rows[len(rows)-1]
		}
		last.cards = append(last.cards, card)
	}
	return rows
}

func timelineDateGroup(date, now time.Time) string {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case !date.Before(today):
		return "今天"
	case !date.Before(today.AddDate(0, 0, -6)):
		return "近 7 天"
	default:
		return "更早"
	}
}

const timelineCardGap float32 = 12

func timelineGrid(width float32) (int, float32) {
	const minWidth, maxWidth float32 = 154, 250
	columns := max(1, int((width+timelineCardGap)/(minWidth+timelineCardGap)))
	return columns, min(maxWidth, (width-float32(columns-1)*timelineCardGap)/float32(columns))
}

func timelineHeading(c *ui.Context, title string) {
	ui.Row(c).FillWidth().Padding(7, 0, 9, 0).Height(36).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).Role(ui.RoleHeading).Level(2).FontSize(12).FontWeight(600).TextColor(c.Theme().TextMuted)
		ui.Box(c).Grow(1).Height(1).Background(c.Theme().SurfacePressed)
	})
}

func (a *desktop) canvasTimelinePage(c *ui.Context) {
	t := &a.canvasTimeline
	width, _ := c.Size()
	width = max(1, width-a.sidebarWidth()-contentInset-2*pageInset)
	columns, cardWidth := timelineGrid(width)

	ui.Column(c.Key(allCanvases)).Fill().Radius(10).Clip().Background(c.Theme().Background).Label("全部画布画廊").Children(func() {
		var cards []canvasTimelineCard
		pageHeader(c, allCanvases, "返回当前画布", func() { a.openWorkspace("画布") }, func() {
			searchWidth := min(float32(164), max(float32(76), width-150))
			if quietSearch(c, &t.query, &t.search, "搜索全部画布", "搜索画布", searchWidth) {
				t.list.ScrollTo(0, ui.Start)
			}
			if sortMenu(c, "全部画布排序", &t.order, timelineSortOptions) {
				t.list.ScrollTo(0, ui.Start)
			}
			cards = a.canvasTimelineCards()
			ui.Textf(c, "%d", len(cards)).FontFeatures("tnum").FontSize(11).TextColor(c.Theme().TextMuted)
		})
		results := ui.Column(c.Key("timeline-results")).Grow(1).FillWidth().MinHeight(0)
		resultsMotion(c, results, resultsMotionKey{query: t.query, order: t.order, count: len(cards)}).Children(func() {
			if len(cards) == 0 {
				ui.Column(c).Fill().Center().Gap(8).Children(func() {
					scene, label := "canvas", "暂无画布"
					if strings.TrimSpace(t.query) != "" {
						scene, label = "search-empty", "没有匹配的画布"
					}
					emptyIcon(c, scene)
					ui.Text(c, label).FontSize(12).TextColor(c.Theme().TextMuted)
				})
				return
			}

			rows := canvasTimelineRows(cards, t.order, columns, time.Now())
			t.list.KeepOffset = true
			t.list.Key = func(i int) any { return rows[i].key }
			list := ui.List(c.Key("canvas-timeline"), &t.list, len(rows), func(i int) {
				row := rows[i]
				if row.title != "" {
					timelineHeading(c, row.title)
					return
				}
				ui.Row(c).FillWidth().Gap(timelineCardGap).Padding(0, 0, 10, 0).Children(func() {
					for _, card := range row.cards {
						a.canvasTimelineCard(c, card, cardWidth)
					}
				})
			})
			thinScroll(c, list.Label("全部画布滚动区"), nil).FillWidth().Grow(1).MinHeight(0).Padding(4, pageInset, 12, pageInset)
		})
	})
}

func (a *desktop) canvasTimelineCard(c *ui.Context, card canvasTimelineCard, width float32) {
	e := card.editor
	p := &e.Project
	date := "未标注日期"
	if !card.date.IsZero() {
		date = card.date.In(time.Local).Format("2006-01-02 15:04")
	} else if a.canvasTimeline.order == timelineOpened {
		date = "未记录打开时间"
	}
	b := ui.ButtonBase(c.Key(p.ID)).Label("查看画布："+p.Name).Column().Width(width).Shrink(0).Padding(0, 0, 4, 0).Gap(2).Radius(7)
	commandHint(c, feedback(c, b, ui.Transparent, c.Theme().TextMuted), "打开画布").Children(func() {
		a.canvasTimelineCover(c, e, width/1.56)
		ui.Row(c).FillWidth().Height(23).Padding(0, 3).Gap(6).AlignItems(ui.Center).Children(func() {
			fittingLabel(c, p.Name, 12, 500).Grow(1)
			ui.Text(c, fmt.Sprintf("%d 张", p.imageNodeCount())).FontSize(10).FontFeatures("tnum").TextColor(c.Theme().TextMuted)
		})
		ui.Text(c, date).SingleLine().FillWidth().Padding(0, 3).FontSize(10).FontFeatures("tnum").TextColor(c.Theme().TextMuted)
	}).OnClick(func() { a.chooseSession(e) })
}

func (a *desktop) canvasTimelineCover(c *ui.Context, e *Editor, height float32) {
	var photos []Asset
	for _, n := range e.Project.Graph.Nodes {
		if asset, ok := e.Assets[n.Asset]; ok && !slices.ContainsFunc(photos, func(p Asset) bool { return p.ID == asset.ID }) {
			photos = append(photos, asset)
			if len(photos) == 3 {
				break
			}
		}
	}
	cover := ui.Box(c).FillWidth().Height(height).Shrink(0).Center().Clip().Radius(7).Background(c.Theme().Surface)
	if len(photos) == 0 {
		cover.Children(func() {
			if len(e.Project.Graph.Nodes) == 0 {
				uiIcon(c, "canvas", 22).TextColor(c.Theme().TextMuted)
				return
			}
			for _, n := range e.Project.Graph.Nodes {
				if n.Kind == textNodeKind {
					ui.Text(c, n.Config.Text).MaxLines(3).FontSize(12).TextColor(c.Theme().TextMuted).Padding(10)
					return
				}
			}
			ui.Box(c).Size(height*.92, height*.76).Center().Radius(4).Background(c.Theme().SurfaceHover).Children(func() {
				uiIcon(c, "spark", 17).TextColor(c.Theme().TextMuted)
			})
		})
		return
	}
	_, windowHeight := c.Size()
	cover.Draw(func(p *ui.Painter, r ui.Rect) {
		const inset, gap float32 = 6, 3
		w, h := r.W-2*inset, r.H-2*inset
		if w <= 0 || h <= 0 || r.Y+r.H <= workspaceHeaderHeight || r.Y >= windowHeight {
			return
		}
		large := ui.Rect{X: r.X + inset, Y: r.Y + inset, W: w, H: h}
		if len(photos) > 1 {
			large.W = (w - gap) * .59
		}
		small := ui.Rect{X: large.X + large.W + gap, Y: large.Y, W: w - large.W - gap, H: h}
		if len(photos) == 3 {
			small.H = (h - gap) / 2
		}
		for i, asset := range photos {
			box := large
			if i > 0 {
				box = small
				box.Y += float32(i-1) * (small.H + gap)
			}
			p.Fill(box, p.Theme().SurfacePressed, 3)
			if a.images != nil {
				a.images.Request(asset, 512)
				if bmp := a.images.Get(asset, 512); bmp != nil {
					p.Clip(box, 3, func() { p.Image(bmp, box, ui.Cover) })
				}
			}
		}
	})
}
