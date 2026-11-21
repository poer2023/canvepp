package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type pictureTimelineRow struct {
	key, title string
	pictures   []libraryEntry
}

func pictureTimelineRows(images []libraryEntry, columns int, now time.Time) []pictureTimelineRow {
	var rows []pictureTimelineRow
	group := ""
	for _, image := range images {
		date, _ := time.Parse(time.RFC3339Nano, image.date)
		title := timelineDateGroup(date, now)
		if group != title {
			rows = append(rows, pictureTimelineRow{key: "heading/" + title, title: title})
			group = title
		}
		last := &rows[len(rows)-1]
		if last.title != "" || len(last.pictures) >= columns {
			rows = append(rows, pictureTimelineRow{key: "pictures/" + image.key})
			last = &rows[len(rows)-1]
		}
		last.pictures = append(last.pictures, image)
	}
	return rows
}

func (a *desktop) overviewTimeline(c *ui.Context, width float32) {
	columns, cardWidth := timelineGrid(width)
	o := a.pictureOverview()
	rows := pictureTimelineRows(o.entries, columns, time.Now())
	list := &o.timeline
	list.KeepOffset = true
	list.Key = func(i int) any { return rows[i].key }
	_, windowHeight := c.Size()
	thinScroll(c, ui.List(c.Key("picture-timeline"), list, len(rows), func(i int) {
		row := rows[i]
		if row.title != "" {
			timelineHeading(c, row.title)
			return
		}
		ui.Row(c).FillWidth().Gap(timelineCardGap).Padding(0, 0, 10, 0).Children(func() {
			for _, item := range row.pictures {
				a.overviewPictureButton(c, item).Column().Width(cardWidth).Shrink(0).Padding(0, 0, 4, 0).Gap(2).Radius(7).Children(func() {
					cover := ui.Box(c).FillWidth().Height(cardWidth / 1.56).Shrink(0).Clip().Radius(7).Background(c.Theme().Surface)
					a.overviewThumbnail(cover, *item.asset, windowHeight)
					ui.Row(c).FillWidth().Height(23).Padding(0, 3).AlignItems(ui.Center).Children(func() {
						fittingLabel(c, item.name, 12, 500).Grow(1)
					})
					ui.Text(c, imageTime(item.asset.CreatedAt)).SingleLine().FillWidth().Padding(0, 3).
						FontSize(10).FontFeatures("tnum").TextColor(c.Theme().TextMuted)
				})
			}
		})
	}).Label("全部图片时间线"), nil).FillWidth().Grow(1).MinHeight(0).Padding(4, pageInset, 12, pageInset)
}
