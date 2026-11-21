package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fundingSource string

const (
	codexFunding  fundingSource = "codex"
	apiFunding    fundingSource = "api"
	apiImageModel               = "gpt-image-2.5-sunburst"
)

func (f fundingSource) subscription() bool { return f == "" || f == codexFunding }
func (f fundingSource) label() string {
	if f.subscription() {
		return "ChatGPT 订阅"
	}
	if f == apiFunding {
		return "OpenAI API"
	}
	return "未知额度"
}

type fundingSettings struct {
	Image               fundingSource `json:"image"`
	Summary             fundingSource `json:"summary"`
	keyDraft, apiStatus string
	apiNote, apiError   string
	busy, inspected     bool
	keyEditing, keyMenu bool
	keyRemoveConfirm    bool
}

func (f *fundingSettings) load(s *Store) error {
	f.Image, f.Summary = codexFunding, codexFunding
	var image, summary string
	err := s.DB.QueryRow("SELECT json_extract(value,'$.image'), json_extract(value,'$.summary') FROM native_meta WHERE key='ai_funding'").Scan(&image, &summary)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	f.Image, f.Summary = fundingSource(image), fundingSource(summary)
	return nil
}
func (a *desktop) chooseFunding(summary bool, next fundingSource) {
	if a.store == nil || (next != apiFunding && next != codexFunding) {
		return
	}
	f := a.funding
	if summary {
		f.Summary = next
	} else {
		f.Image = next
	}
	if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('ai_funding',?)", string(encode(f))); err != nil {
		a.message(err.Error())
		return
	}
	a.funding.Image, a.funding.Summary = f.Image, f.Summary
}
func (a *desktop) fundingRow(c *ui.Context, title string, summary bool) {
	selected := a.funding.Image
	icon := "picture"
	if summary {
		selected = a.funding.Summary
		icon = "text-lines"
	}
	if selected.subscription() {
		selected = codexFunding
	}
	settingsRow(c, title, icon, func() {
		options := []settingsOption[fundingSource]{
			{codexFunding, codexFunding.label(), "subscription"},
			{apiFunding, apiFunding.label(), "plug"},
		}
		if settingsSelect(c, title+"来源", &selected, options, selected.label()).Changed() {
			a.chooseFunding(summary, selected)
		}
	})
}

func (f *fundingSettings) closeKeyEditor() {
	f.keyDraft = ""
	f.keyEditing, f.keyMenu, f.keyRemoveConfirm = false, false, false
}

func (a *desktop) inspectAPIKey() {
	f := &a.funding
	if a.store == nil || f.busy {
		return
	}
	f.inspected = true
	f.apiStatus = ""
	go func() {
		_, err := a.store.openAI.key()
		a.publishSettings(func() {
			f.apiError = ""
			switch {
			case err == nil:
				f.apiStatus = "已保存"
			case errors.Is(err, errAPIKeyMissing):
				f.apiStatus = "未配置"
			default:
				f.apiStatus, f.apiError = "状态未知", err.Error()
			}
		})
	}()
}

func (a *desktop) apiSettings(c *ui.Context) {
	f := &a.funding
	if !f.inspected && a.settingsWin != nil {
		a.inspectAPIKey()
	}
	settingsRow(c, "OpenAI API Key", "key", func() {
		switch {
		case f.busy:
			settingsStatus(c, "处理中…", settingsNeutral)
		case f.apiStatus == "":
			settingsStatus(c, "正在读取密钥…", settingsNeutral)
		case f.apiStatus == "状态未知":
			settingsStatus(c, f.apiStatus, settingsWarning)
			iconButton(c, "refresh", "重新读取密钥状态", a.inspectAPIKey)
		case f.apiStatus == "未配置":
			tone := settingsNeutral
			if f.Image == apiFunding || f.Summary == apiFunding {
				tone = settingsWarning
			}
			settingsStatus(c, f.apiStatus, tone)
			if !f.keyEditing {
				settingsButton(c, "添加密钥", func() { f.keyEditing = true })
			}
		default:
			tone := settingsReady
			if f.apiStatus == "已配置" {
				tone = settingsWarning
			}
			settingsStatus(c, f.apiStatus, tone)
			if !f.keyEditing {
				a.apiKeyMenu(c)
			}
		}
	})
	if f.keyEditing {
		a.apiKeyEditor(c)
	}
	settingsMessage(c, f.apiNote, settingsWarning)
	settingsMessage(c, f.apiError, settingsError)
}

func (a *desktop) apiKeyEditor(c *ui.Context) {
	t := c.Theme()
	f := &a.funding
	ui.Row(c.Key("edit-api-key")).FillWidth().Padding(6, 0).Gap(7).
		AlignItems(ui.Center).Background(t.Background).Children(func() {
		field := ui.TextInput(c.Key("api-key-field"), &f.keyDraft).Password().Label("OpenAI API Key 输入").
			Placeholder("输入 API Key").Grow(1).MinWidth(0).Height(32).Padding(4, 10).FontSize(12).
			Radius(6).Background(t.Background).Border(1, t.Border).AutoFocus()
		flatTip(c, disable(field, f.busy), "密钥仅保存在本机系统凭据库")
		disable(settingsButton(c, "取消", f.closeKeyEditor), f.busy)
		save := ui.PrimaryButton(c.Key("save-api-key"), "保存").Label("保存 API Key").
			Height(32).Padding(0, 12).Radius(6).FontSize(12).Border(0, ui.Transparent).Shrink(0)
		disable(feedback(c, save, t.Accent, t.AccentText), f.busy || strings.TrimSpace(f.keyDraft) == "").
			OnClick(func() { a.verifyAPI(true) })
	})
}

func (a *desktop) apiKeyMenu(c *ui.Context) {
	t := c.Theme()
	f := &a.funding
	anchor := iconButton(c, "more", "管理密钥", func() {
		f.keyMenu = !f.keyMenu
		f.keyRemoveConfirm = false
	})
	ui.PopoverBase(c, anchor, &f.keyMenu, func(panel ui.Element) {
		toolSurface(c, panel, 8).Background(t.Background).Border(1, t.Border).
			AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(4, 0, 0, 0)
		if f.keyRemoveConfirm {
			ui.Column(c).Width(240).Padding(11).Gap(8).Children(func() {
				ui.Text(c, "移除此设备保存的 OpenAI API Key？").FontSize(13).FontWeight(600)
				ui.Text(c, "使用此密钥的生图和摘要需重新配置。").FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Justify(ui.End).Gap(6).Children(func() {
					settingsButton(c, "取消", f.closeKeyEditor).AutoFocus()
					settingsButton(c, "移除", a.removeAPIKey).TextColor(t.Danger)
				})
			})
			return
		}
		ui.Column(c).Width(150).Padding(4).Gap(2).Children(func() {
			settingsButton(c, "更换密钥", func() {
				f.keyMenu, f.keyEditing = false, true
			}).FillWidth().Justify(ui.Start)
			settingsButton(c, "检查可用性", func() {
				f.keyMenu = false
				a.verifyAPI(false)
			}).FillWidth().Justify(ui.Start)
			settingsButton(c, "移除密钥…", func() {
				f.keyRemoveConfirm = true
			}).FillWidth().Justify(ui.Start).TextColor(t.Danger)
		})
	})
}

func (a *desktop) removeAPIKey() {
	if a.store == nil || a.funding.busy {
		return
	}
	a.funding.busy = true
	a.funding.apiError = ""
	a.funding.closeKeyEditor()
	go func() {
		err := a.store.openAI.secrets.Delete(a.store.openAI.secretID)
		a.publishSettings(func() {
			f := &a.funding
			f.busy = false
			if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
				f.apiError = "系统凭据库移除失败"
				return
			}
			f.keyDraft, f.apiNote = "", ""
			f.apiStatus = "未配置"
		})
	}()
}

func (a *desktop) verifyAPI(save bool) {
	if a.store == nil || a.funding.busy {
		return
	}
	f := &a.funding
	draft := strings.TrimSpace(f.keyDraft)
	if save && draft == "" {
		return
	}
	f.busy, f.apiError = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		client := a.store.openAI
		key := draft
		var err error
		if !save {
			key, err = client.key()
		}
		status := ""
		if err == nil {
			status, err = client.verify(ctx, key)
		}
		if err == nil && save {
			if client.secrets.Set(client.secretID, []byte(key)) != nil {
				err = errors.New("系统凭据库保存失败")
			}
		}
		a.publishSettings(func() {
			f.busy = false
			if err != nil {
				if errors.Is(err, errAPIKeyMissing) {
					f.apiStatus, f.apiNote = "未配置", ""
				} else {
					f.apiError = err.Error()
				}
				a.message(err.Error())
				return
			}
			f.apiStatus, f.apiNote = "已验证", ""
			if status != "已验证" {
				f.apiStatus, f.apiNote = "已配置", "密钥缺少模型列表读取权限。"
			}
			a.message(status)
			if save {
				f.closeKeyEditor()
			}
		})
	}()
}
func summarizeWithFunding(ctx context.Context, s *Store, source fundingSource, model modelChoice, events string) (textReply, error) {
	if source == apiFunding {
		return s.openAI.summary(ctx, model.withDefault(textModel), events)
	}
	if !source.subscription() {
		return textReply{}, errors.New("未知的摘要额度来源")
	}
	return requestCodexText(ctx, s, model, "", logInstructions, events, true, nil)
}
