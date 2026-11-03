package status

import (
	"fmt"
	"strings"

	"github.com/abhirupghosh/quota/internal/identity"
)

// AccountOption is a named snapshot the menu can apply. Keep this type here
// so status does not import switcher.
type AccountOption struct {
	Name  string
	Email string
	Tool  string
}

func FormatSwiftBar(r Report, accounts []AccountOption, bin string) string {
	c, x, g := macPercents(r)
	worst := maxNonEmpty(c, x, g)
	if short := macShortWorst(r); short != "" {
		worst = short
	}
	title := worst
	if title == "" {
		title = "·"
	}
	var b strings.Builder
	b.WriteString(sb(title,
		"font=Menlo",
		"size=11",
		"sfimage="+titleSymbol(worst),
		"sfsize=14",
		"color="+levelColor(worst),
	))
	b.WriteByte('\n')
	b.WriteString("---\n")

	claude := claudeOptions(accounts)
	home := homeTag(r)
	for _, h := range r.Hosts {
		if h.Err != "" {
			b.WriteString(sb(fmt.Sprintf("%-7s  %s", h.Host, h.Err),
				"font=Menlo", "size=12", "color=#E85D5D", "disabled=true"))
			b.WriteByte('\n')
			continue
		}
		line, worstHost := hostLine(h, home)
		b.WriteString(sb(line, "font=Menlo", "size=13", "color="+levelColor(worstHost)))
		b.WriteByte('\n')
		writeClaudeMenu(&b, h, claude, bin)
	}

	b.WriteString("---\n")
	b.WriteString(sb("Refresh", "refresh=true", "sfimage=arrow.clockwise"))
	b.WriteByte('\n')
	return b.String()
}

func writeClaudeMenu(b *strings.Builder, h HostReport, opts []AccountOption, bin string) {
	if len(opts) == 0 || bin == "" {
		return
	}
	current := currentEmail(h, identity.Claude)
	b.WriteString(sb("-- Switch Claude  (next session)", "size=11"))
	b.WriteByte('\n')
	for _, o := range opts {
		label := "---- " + o.Name
		if tag := shortAccount(o.Email); tag != "" && tag != o.Name {
			label += " · " + tag
		}
		params := []string{"font=Menlo", "size=12"}
		if current != "" && strings.EqualFold(current, o.Email) {
			label += "  (live)"
			params = append(params, "checked=true", "disabled=true")
		} else {
			params = append(params, switchParams(bin, h.Host, o.Name)...)
		}
		b.WriteString(sb(label, params...))
		b.WriteByte('\n')
	}
}

func switchParams(bin, host, name string) []string {
	// SwiftBar runs: bash param1 param2 ...
	p := []string{
		"bash=" + bin,
		"param1=use",
		"param2=claude",
		"param3=" + name,
	}
	if host != "" && host != "mac" {
		p = append(p, "param4=--host", "param5="+host)
	}
	p = append(p, "terminal=false", "refresh=true")
	return p
}

func currentEmail(h HostReport, tool identity.Tool) string {
	for _, t := range h.Tools {
		if t.Tool == tool {
			return t.Email
		}
	}
	return ""
}

func claudeOptions(accounts []AccountOption) []AccountOption {
	var out []AccountOption
	for _, a := range accounts {
		if a.Tool == "" || a.Tool == "claude" {
			out = append(out, a)
		}
	}
	return out
}

func hostLine(h HostReport, home string) (string, string) {
	byTool := map[identity.Tool]ToolRow{}
	for _, t := range h.Tools {
		byTool[t.Tool] = t
	}
	c := bitsFor(byTool[identity.Claude], "C")
	x := bitsFor(byTool[identity.Codex], "X")
	g := bitsFor(byTool[identity.Grok], "G")
	note := accountNote(h, home)
	line := fmt.Sprintf("%-7s  %-7s  %-5s  %-4s", h.Host, c.text, x.text, g.text)
	if note != "" {
		line += "  " + note
	}
	return line, maxNonEmpty(c.worst, x.worst, g.worst)
}

type toolBits struct {
	text  string
	worst string
}

func bitsFor(t ToolRow, letter string) toolBits {
	if t.Tool == "" && t.Email == "" && t.Err == "" {
		return toolBits{text: letter + "—"}
	}
	if t.Email == "" && t.Err != "" {
		return toolBits{text: letter + "—"}
	}
	var parts []string
	for _, w := range t.Usage.Windows {
		if w.Utilization == nil {
			continue
		}
		v := *w.Utilization
		if v > 0 && v <= 1 {
			v *= 100
		}
		// Skip empty extra lanes (Spark at 0%) so the row stays short.
		if w.Label != "5h" && w.Label != "wk" && w.Label != "credits" && v < 1 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%.0f", v))
	}
	if len(parts) == 0 {
		if strings.Contains(t.Usage.Err, "429") {
			return toolBits{text: letter + "?"}
		}
		return toolBits{text: letter + "—", worst: worstWindow(t)}
	}
	return toolBits{text: letter + strings.Join(parts, "/"), worst: worstWindow(t)}
}

// homeTag is the short tag of this Mac's Claude account. Rows only call out
// accounts that differ from it, so the common case stays quiet.
func homeTag(r Report) string {
	for _, h := range r.Hosts {
		if h.Host == "mac" {
			return shortAccount(currentEmail(h, identity.Claude))
		}
	}
	return ""
}

func accountNote(h HostReport, home string) string {
	var tags []string
	seen := map[string]bool{}
	for _, t := range h.Tools {
		tag := shortAccount(t.Email)
		if tag == "" || tag == home || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return strings.Join(tags, ",")
}

// personalDomains map consumer mail hosts to a short tag. Any other domain is
// treated as an org and tagged by its first label (you@acme.io → "acme").
var personalDomains = map[string]string{
	"gmail.com":      "gmail",
	"googlemail.com": "gmail",
	"icloud.com":     "icloud",
	"me.com":         "icloud",
	"outlook.com":    "outlook",
	"hotmail.com":    "outlook",
	"live.com":       "outlook",
	"proton.me":      "proton",
	"protonmail.com": "proton",
	"yahoo.com":      "yahoo",
}

func shortAccount(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ""
	}
	i := strings.LastIndexByte(email, '@')
	if i < 0 {
		return email
	}
	domain := email[i+1:]
	if tag, ok := personalDomains[domain]; ok {
		return tag
	}
	if j := strings.IndexByte(domain, '.'); j > 0 {
		return domain[:j]
	}
	if domain != "" {
		return domain
	}
	return email[:i]
}

func macShortWorst(r Report) string {
	var vals []string
	for _, h := range r.Hosts {
		if h.Host != "mac" {
			continue
		}
		for _, t := range h.Tools {
			for _, w := range t.Usage.Windows {
				if w.Utilization == nil {
					continue
				}
				if w.Label != "5h" && w.Label != "credits" {
					continue
				}
				v := *w.Utilization
				if v > 0 && v <= 1 {
					v *= 100
				}
				vals = append(vals, fmt.Sprintf("%.0f", v))
			}
		}
	}
	return maxNonEmpty(vals...)
}

func macPercents(r Report) (c, x, g string) {
	for _, h := range r.Hosts {
		if h.Host != "mac" {
			continue
		}
		for _, t := range h.Tools {
			p := primaryPct(t)
			switch t.Tool {
			case identity.Claude:
				c = p
			case identity.Codex:
				x = p
			case identity.Grok:
				g = p
			}
		}
	}
	return
}

func worstWindow(t ToolRow) string {
	var best float64
	found := false
	for _, w := range t.Usage.Windows {
		if w.Utilization == nil {
			continue
		}
		v := *w.Utilization
		if v > 0 && v <= 1 {
			v *= 100
		}
		if !found || v > best {
			best = v
			found = true
		}
	}
	if !found {
		return ""
	}
	return fmt.Sprintf("%.0f", best)
}

func levelColor(pct string) string {
	if pct == "" || pct == "—" {
		return "#8E8E93"
	}
	var v float64
	if _, err := fmt.Sscanf(pct, "%f", &v); err != nil {
		return "#8E8E93"
	}
	switch {
	case v >= 90:
		return "#E85D5D"
	case v >= 70:
		return "#E0A106"
	default:
		return "#34C759"
	}
}

func titleSymbol(pct string) string {
	if pct == "" || pct == "—" {
		return "gauge.with.dots.needle.33percent"
	}
	var v float64
	fmt.Sscanf(pct, "%f", &v)
	switch {
	case v >= 90:
		return "gauge.with.dots.needle.67percent"
	case v >= 50:
		return "gauge.with.dots.needle.50percent"
	default:
		return "gauge.with.dots.needle.33percent"
	}
}

func maxNonEmpty(vals ...string) string {
	best := ""
	bestV := -1.0
	for _, s := range vals {
		if s == "" || s == "—" {
			continue
		}
		var v float64
		if _, err := fmt.Sscanf(s, "%f", &v); err != nil {
			continue
		}
		if v > bestV {
			bestV = v
			best = s
		}
	}
	return best
}

func sb(text string, params ...string) string {
	text = strings.ReplaceAll(text, "|", "¦")
	if len(params) == 0 {
		return text
	}
	return text + " | " + strings.Join(params, " ")
}
