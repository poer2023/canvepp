package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"
)

const (
	imageModelID  = "gpt-6.2-sol"
	textModel     = "gpt-6.1-terra"
	defaultEffort = "medium"
)

type modelChoice struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

func (m modelChoice) withDefault(id string) modelChoice {
	if m.Model == "" {
		m.Model = id
	}
	if m.Effort == "" {
		m.Effort = defaultEffort
	}
	return m
}

type modelSettings struct {
	Image modelChoice `json:"image"`
	Text  modelChoice `json:"text"`
}

func (m *modelSettings) load(s *Store) error {
	var raw string
	switch err := s.DB.QueryRow("SELECT value FROM native_meta WHERE key='ai_models'").Scan(&raw); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	default:
		var saved modelSettings
		if json.Unmarshal([]byte(raw), &saved) == nil {
			*m = saved
		}
	}
	m.Image, m.Text = m.Image.withDefault(imageModelID), m.Text.withDefault(textModel)
	return nil
}

func (a *desktop) saveModelChoice(image bool, choice modelChoice) {
	if a.store == nil {
		return
	}
	models := a.models
	if image {
		models.Image = choice
	} else {
		models.Text = choice
	}
	if _, err := a.store.DB.Exec("INSERT OR REPLACE INTO native_meta VALUES('ai_models',?)", string(encode(models))); err != nil {
		a.message("模型设置未保存 " + err.Error())
		return
	}
	a.models = models
}

var gptModelOptions = []settingsOption[string]{
	{imageModelID, "GPT-6.2 Sol", "spark"},
	{"gpt-6.2-astra", "GPT-6.2 Astra", "spark"},
	{textModel, "GPT-6.1 Terra", "spark"},
	{"gpt-6-luna", "GPT-6 Luna", "spark"},
}

var effortOptions = []settingsOption[string]{
	{"low", "低", "sliders"},
	{defaultEffort, "中等", "sliders"},
	{"high", "高", "sliders"},
	{"xhigh", "超高", "sliders"},
	{"max", "最大", "sliders"},
	{"ultra", "极限", "sliders"},
}

func effortsForModel(model string) []settingsOption[string] {
	if model == "gpt-6-luna" {
		return effortOptions[:len(effortOptions)-1]
	}
	return effortOptions
}

func (a *desktop) modelRows(c *ui.Context, image bool) {
	choice, label := a.models.Text.withDefault(textModel), "文本"
	if image {
		if a.funding.Image == apiFunding {
			settingsRow(c, "生图模型", "picture", func() { settingsStatus(c, "GPT Image 2.5", settingsNeutral) })
			return
		}
		choice, label = a.models.Image.withDefault(imageModelID), "生图"
	}
	settingsRow(c, label+"模型", "spark", func() {
		if settingsSelect(c, "选择"+label+"模型", &choice.Model, gptModelOptions, choice.Model).Changed() {
			if choice.Model == "gpt-6-luna" && choice.Effort == "ultra" {
				choice.Effort = defaultEffort
			}
			a.saveModelChoice(image, choice)
		}
	})
	settingsRow(c, label+"推理强度", "sliders", func() {
		if settingsSelect(c, "选择"+label+"推理强度", &choice.Effort, effortsForModel(choice.Model), choice.Effort).Changed() {
			a.saveModelChoice(image, choice)
		}
	})
}

func (c *Codex) requireModel(ctx context.Context, choice modelChoice) error {
	cursor := ""
	for range 100 {
		var models struct {
			Data []struct {
				Model   string `json:"model"`
				Efforts []struct {
					Value string `json:"reasoningEffort"`
				} `json:"supportedReasoningEfforts"`
			} `json:"data"`
			Next string `json:"nextCursor"`
		}
		params := map[string]any{"limit": 100, "includeHidden": false}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.Call(ctx, "model/list", params, &models); err != nil {
			return err
		}
		for _, m := range models.Data {
			if m.Model != choice.Model {
				continue
			}
			if len(m.Efforts) == 0 {
				return nil
			}
			for _, allowed := range m.Efforts {
				if allowed.Value == choice.Effort {
					return nil
				}
			}
			return fmt.Errorf("%s 当前不支持推理强度 %s", choice.Model, choice.Effort)
		}
		if models.Next == "" || models.Next == cursor {
			break
		}
		cursor = models.Next
	}
	return fmt.Errorf("当前 Codex 账户未提供 %s，可在设置中选择模型", choice.Model)
}
