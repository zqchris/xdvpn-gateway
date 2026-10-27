package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

const companyRulesPath = "/etc/xdvpn/company-rules.txt"

type companyRules struct{ Networks, Domains []string }

func parseCompanyRules(r io.Reader) (companyRules, error) {
	var out companyRules
	b, e := io.ReadAll(io.LimitReader(r, (256<<10)+1))
	if e != nil || len(b) > 256<<10 {
		return out, errors.New("company rules unreadable or too large")
	}
	seen := map[string]bool{}
	for i, line := range strings.Split(strings.TrimPrefix(string(b), "\ufeff"), "\n") {
		line = strings.SplitN(line, "#", 2)[0]
		line = strings.NewReplacer("，", " ", ",", " ", "；", " ", ";", " ").Replace(line)
		for _, v := range strings.Fields(line) {
			v = strings.ToLower(strings.TrimSuffix(v, "."))
			key := ""
			if ip := net.ParseIP(v); ip != nil && ip.To4() != nil {
				key = ip.String() + "/32"
			} else if _, n, e := net.ParseCIDR(v); e == nil && n.IP.To4() != nil {
				ones, _ := n.Mask.Size()
				if ones > 0 {
					key = n.String()
				}
			}
			if key != "" {
				if !seen[key] {
					out.Networks = append(out.Networks, key)
					seen[key] = true
				}
			} else {
				if !validDomain(v) || strings.Trim(v, "0123456789.") == "" {
					return companyRules{}, fmt.Errorf("invalid company rule at line %d", i+1)
				}
				if !seen[v] {
					out.Domains = append(out.Domains, v)
					seen[v] = true
				}
			}
			if len(out.Networks) > 4096 || len(out.Domains) > 256 {
				return companyRules{}, errors.New("company rule limit exceeded")
			}
		}
	}
	if len(out.Networks)+len(out.Domains) == 0 {
		return out, errors.New("company rules are empty")
	}
	return out, nil
}
func loadCompanyRules() (companyRules, error) {
	f, e := os.Open(companyRulesPath)
	if e != nil {
		return companyRules{}, errors.New("company rules file unavailable")
	}
	defer f.Close()
	return parseCompanyRules(f)
}
func policyMode(s string) string {
	if s == "server" {
		return "company"
	}
	return s
}
func policyTargets(mode, manual string) (string, []string, error) {
	switch policyMode(mode) {
	case "networks":
		return manual, nil, nil
	case "global4":
		return "", nil, nil
	case "company":
		rules, e := loadCompanyRules()
		if e != nil {
			return "", nil, e
		}
		return strings.TrimSpace(manual + " " + strings.Join(rules.Domains, " ")), rules.Networks, nil
	default:
		return "", nil, errors.New("invalid traffic mode")
	}
}
func companyInfo() error {
	r, e := loadCompanyRules()
	if e != nil {
		return emitUI(map[string]any{"valid": false, "error": "预设清单缺失、为空或格式错误；请检查路由器本地文件"})
	}
	return emitUI(map[string]any{"valid": true, "ip_count": len(r.Networks), "domain_count": len(r.Domains)})
}
