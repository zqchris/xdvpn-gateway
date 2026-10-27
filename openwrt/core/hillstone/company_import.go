package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Fixed destination, validated before replacement; never log or echo private content.
func storeCompanyRules(path, content string) (companyRules, error) {
	rules, err := parseCompanyRules(strings.NewReader(content))
	if err != nil {
		return rules, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return rules, errors.New("cannot create private directory")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".company-rules-*")
	if err != nil {
		return rules, errors.New("cannot create private file")
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(content); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return rules, errors.New("cannot write private file")
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return rules, errors.New("cannot replace private file")
	}
	return rules, nil
}

func importCompanyRules() error {
	var req struct {
		Content string `json:"content"`
	}
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 2<<20))
	if err := decoder.Decode(&req); err != nil {
		return emitUI(map[string]any{"error": "无法读取规则文件，请使用 UTF-8 文本，最大 256 KiB"})
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return emitUI(map[string]any{"error": "导入请求格式错误"})
	}
	rules, err := storeCompanyRules(companyRulesPath, req.Content)
	if err != nil {
		return emitUI(map[string]any{"error": "导入失败，原清单保留：" + err.Error()})
	}
	refresh := "offline"
	if b, e := exec.Command("/usr/libexec/xdvpn/reload-rules").Output(); e == nil {
		var result struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(b, &result) == nil && result.Status == "refresh_requested" {
			refresh = result.Status
		}
	} else {
		refresh = "refresh_failed"
	}
	return emitUI(map[string]any{"valid": true, "ip_count": len(rules.Networks), "domain_count": len(rules.Domains), "refresh": refresh})
}
