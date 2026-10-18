package main

import (
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"
)

type theme struct {
	bg, border, fg, muted, faint string
	add, del, line, track, brass string
	langs                        []string // colors for the top languages
	other                        string
}

// GitHub's own palette, so the widget looks native in both themes.
var (
	darkTheme = theme{
		bg: "#0d1117", border: "#30363d", fg: "#e6edf3", muted: "#8b949e", faint: "#6e7681",
		add: "#3fb950", del: "#f85149", line: "#21262d", track: "#21262d", brass: "#d4a85a",
		langs: []string{"#58a6ff", "#d29922", "#a371f7"}, other: "#6e7681",
	}
	lightTheme = theme{
		bg: "#ffffff", border: "#d0d7de", fg: "#1f2328", muted: "#59636e", faint: "#818b98",
		add: "#1a7f37", del: "#cf222e", line: "#d8dee4", track: "#eaeef2", brass: "#9a6700",
		langs: []string{"#0969da", "#bf8700", "#8250df"}, other: "#818b98",
	}
)

const (
	width = 840
	left  = 40
	right = 800

	nameMax = 16 // chars that fit the name column
	barX    = 186
	barW    = 330
)

func render(s *Snapshot, now time.Time, t theme) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	var touched []RepoStats
	var add, del, commits, loc, locDelta int
	langs := map[string]int{}
	for _, r := range s.Repos {
		if r.Commits > 0 {
			touched = append(touched, r)
		}
		add += r.Additions
		del += r.Deletions
		commits += r.Commits
		loc += r.LOC
		locDelta += r.LOCDelta
		for l, n := range r.Languages {
			langs[l] += n
		}
	}
	quiet := commits == 0

	heading := func(y int, label string) {
		w(`<text x="%d" y="%d" font-size="12" letter-spacing="2.2" fill="%s">CODE <tspan fill="%s">//</tspan> %s</text>`,
			left, y, t.muted, t.faint, label)
	}

	// Today: date line and the four big numbers.
	y := 52
	heading(y, "TODAY")
	w(`<text x="%d" y="%d" text-anchor="end" font-size="12" letter-spacing="1" fill="%s">%s</text>`,
		right, y, t.muted, strings.ToUpper(now.Format("Mon 2006.01.02")))

	y += 52
	big := []struct{ val, label, color string }{
		{"+" + comma(add), "added", t.add},
		{"−" + comma(del), "removed", t.del},
		{signed(add - del), "net", t.fg},
		{comma(commits), "commits", t.fg},
	}
	for i, n := range big {
		color := n.color
		if quiet {
			color = t.faint
		}
		x := left + i*190
		w(`<text x="%d" y="%d" font-size="30" font-weight="700" fill="%s">%s</text>`, x, y, color, n.val)
		w(`<text x="%d" y="%d" font-size="12" fill="%s">%s</text>`, x, y+22, t.muted, n.label)
	}
	y += 22

	// Today: per-repo rows, or the quiet-day box.
	if quiet {
		top := y + 26
		w(`<rect x="%d" y="%d" width="%d" height="66" rx="8" fill="none" stroke="%s" stroke-dasharray="4 4"/>`,
			left, top, right-left, t.border)
		w(`<text x="%d" y="%d">Quiet day. No commits yet.</text>`, left+16, top+27)
		if la := s.LastActive; la != nil {
			day := la.Date
			if d, err := time.Parse(time.DateOnly, la.Date); err == nil {
				day = d.Format("Mon Jan 2")
			}
			w(`<text x="%d" y="%d" font-size="12" fill="%s">last activity %s · %s <tspan fill="%s">+%s</tspan> <tspan fill="%s">−%s</tspan></text>`,
				left+16, top+49, t.muted, esc(day), esc(truncate(la.Repo)), t.add, comma(la.Additions), t.del, comma(la.Deletions))
		}
		y = top + 66 + 26
	} else {
		maxTotal := 1
		for _, r := range touched {
			maxTotal = max(maxTotal, r.Additions+r.Deletions)
		}
		y += 14
		for i, r := range touched {
			y += 28
			addW := barW * r.Additions / maxTotal
			delW := barW * r.Deletions / maxTotal
			w(`<text x="%d" y="%d">%s</text>`, left, y, esc(truncate(r.Name)))
			w(`<clipPath id="today%d"><rect x="%d" y="%d" width="%d" height="6" rx="3"/></clipPath>`, i, barX, y-9, barW)
			w(`<g clip-path="url(#today%d)"><rect x="%d" y="%d" width="%d" height="6" fill="%s"/>`, i, barX, y-9, barW, t.track)
			w(`<rect x="%d" y="%d" width="%d" height="6" fill="%s"/>`, barX, y-9, addW, t.add)
			w(`<rect x="%d" y="%d" width="%d" height="6" fill="%s"/></g>`, barX+addW, y-9, delW, t.del)
			w(`<text x="616" y="%d" text-anchor="end" fill="%s">+%s</text>`, y, t.add, comma(r.Additions))
			w(`<text x="696" y="%d" text-anchor="end" fill="%s">−%s</text>`, y, t.del, comma(r.Deletions))
			w(`<text x="%d" y="%d" text-anchor="end" fill="%s">%s</text>`, right, y, t.muted, plural(r.Commits, "commit"))
		}
		y += 24
	}

	w(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s"/>`, left, y, right, y, t.line)

	// Total: per-repo LOC with change since yesterday, then the sum.
	y += 38
	heading(y, "TOTAL")
	y += 4
	totalRow := func(name string, loc, delta int, langs map[string]int, weight string) {
		arrow, color := deltaLabel(delta, t)
		w(`<text x="%d" y="%d" font-weight="%s">%s</text>`, left, y, weight, esc(truncate(name)))
		w(`<text x="290" y="%d" text-anchor="end" font-weight="%s">%s LOC</text>`, y, weight, comma(loc))
		w(`<text x="316" y="%d" font-weight="%s" fill="%s">%s</text>`, y, weight, color, arrow)
		w(`<text x="446" y="%d" font-size="12" fill="%s">%s</text>`, y, t.muted, esc(shareText(shares(langs))))
	}
	for _, r := range s.Repos {
		y += 28
		totalRow(r.Name, r.LOC, r.LOCDelta, r.Languages, "400")
	}
	y += 32
	totalRow("total", loc, locDelta, langs, "700")

	// Language bar across the full width.
	y += 16
	w(`<clipPath id="langs"><rect x="%d" y="%d" width="%d" height="6" rx="3"/></clipPath><g clip-path="url(#langs)">`,
		left, y, right-left)
	w(`<rect x="%d" y="%d" width="%d" height="6" fill="%s"/>`, left, y, right-left, t.track)
	x := float64(left)
	for i, sh := range shares(langs) {
		color := t.other
		if sh.name != "other" && i < len(t.langs) {
			color = t.langs[i]
		}
		segW := sh.frac * float64(right-left)
		w(`<rect x="%.1f" y="%d" width="%.1f" height="6" fill="%s"/>`, x, y, segW, color)
		x += segW
	}
	w(`</g>`)

	// Footer.
	y += 6 + 40
	w(`<text x="%d" y="%d" font-size="12" font-weight="700" letter-spacing="2.9" fill="%s">FOR THE EMPEROR</text>`,
		left, y, t.brass)
	w(`<text x="%d" y="%d" text-anchor="end" font-size="12" fill="%s">updated %s PT · %s</text>`,
		right, y, t.faint, now.Format("15:04"), now.Format("Jan 2"))
	height := y + 32

	title := fmt.Sprintf("Code activity: +%s / −%s today, %s LOC total", comma(add), comma(del), comma(loc))
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-labelledby="title">
<title id="title">%s</title>
<style>text{font-family:'JetBrains Mono',ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums}</style>
<rect x="0.5" y="0.5" width="%d" height="%d" rx="12" fill="%s" stroke="%s"/>
<g font-size="14" fill="%s">
%s</g>
</svg>
`, width, height, width, height, title, width-1, height-1, t.bg, t.border, t.fg, b.String())
}

type share struct {
	name string
	frac float64
}

// shares returns the top languages (up to 3, at least 1% each) and an
// "other" bucket for the rest.
func shares(langs map[string]int) []share {
	total := 0
	for _, n := range langs {
		total += n
	}
	if total == 0 {
		return nil
	}
	names := make([]string, 0, len(langs))
	for l := range langs {
		names = append(names, l)
	}
	sort.Slice(names, func(i, j int) bool {
		if langs[names[i]] != langs[names[j]] {
			return langs[names[i]] > langs[names[j]]
		}
		return names[i] < names[j]
	})

	var out []share
	rest := 1.0
	for _, l := range names {
		f := float64(langs[l]) / float64(total)
		if len(out) == 3 || f < 0.01 {
			break
		}
		out = append(out, share{l, f})
		rest -= f
	}
	if rest > 0.005 {
		out = append(out, share{"other", rest})
	}
	return out
}

func shareText(sh []share) string {
	parts := make([]string, len(sh))
	for i, s := range sh {
		parts[i] = fmt.Sprintf("%s %d%%", s.name, int(s.frac*100+0.5))
	}
	return strings.Join(parts, " · ")
}

func deltaLabel(n int, t theme) (string, string) {
	switch {
	case n > 0:
		return "↑ +" + comma(n), t.add
	case n < 0:
		return "↓ −" + comma(-n), t.del
	default:
		return "→ 0", t.faint
	}
}

func comma(n int) string {
	if n < 0 {
		return "-" + comma(-n)
	}
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func signed(n int) string {
	switch {
	case n > 0:
		return "+" + comma(n)
	case n < 0:
		return "−" + comma(-n)
	default:
		return "0"
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return comma(n) + " " + word + "s"
}

func truncate(s string) string {
	if r := []rune(s); len(r) > nameMax {
		return string(r[:nameMax-1]) + "…"
	}
	return s
}

func esc(s string) string { return html.EscapeString(s) }
