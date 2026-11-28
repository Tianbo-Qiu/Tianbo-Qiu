package main

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"time"
)

// The big figures use Barlow Condensed SemiBold (SIL OFL, fonts/OFL.txt),
// subset to Latin-1 punctuation and digits and inlined so every viewer sees
// the same face; GitHub serves the SVG as an image, which can't load fonts.
//
//go:embed fonts/barlow-condensed-semibold.woff2
var condensedFont []byte

// One light card: it reads as a printed sheet on GitHub's light and dark
// themes alike, so there is no dark variant.
const (
	width  = 840
	height = 560
	left   = 40
	right  = 800

	paper = "#FAF8F3"
	ink   = "#121212"
	hair  = "#D9D4CA"
	red   = "#D7263D"

	// Token types. Checked as a set against the paper for contrast and
	// color-vision separation; every one also carries a text label.
	gold = "#B38A3E" // cache read
	blue = "#2457B3" // cache write
)

// Advance widths in em of the condensed glyphs the big figures use. Digits
// use the font's tabular figures, so every digit cell is the same width and
// a column can roll through 0–9 without shifting its neighbours.
const digitAdvance = 0.475

var glyphAdvance = map[rune]float64{'+': 0.439, '−': 0.439, ',': 0.199, '.': 0.211, '%': 0.772}

func advance(r rune) float64 {
	if r >= '0' && r <= '9' {
		return digitAdvance
	}
	if a, ok := glyphAdvance[r]; ok {
		return a
	}
	return 0.5
}

func textWidth(s string, size float64) float64 {
	w := 0.0
	for _, r := range s {
		w += advance(r)
	}
	return w * size
}

// fit shrinks size so that width(size) fits in max.
func fit(size, max float64, width func(float64) float64) float64 {
	if w := width(size); w > max {
		return size * max / w
	}
	return size
}

func render(r *Report) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	rule := func(x, y, wd, h int, fill string) {
		w(`<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`, x, y, wd, h, fill)
	}

	// Masthead.
	w(`<circle cx="%d" cy="37" r="3" fill="%s" class="live"/>`, left+3, red)
	w(`<text x="%d" y="42" class="c head">THE DAILY CODE REPORT</text>`, left+14)
	w(`<text x="420" y="42" class="h mid">NO. %d</text>`, r.Now.YearDay())
	w(`<text x="%d" y="42" class="h end">%s PT</text>`, right, strings.ToUpper(r.Now.Format("Mon 2 Jan 2006 · 15:04")))
	rule(left, 54, right-left, 2, ink)
	rule(left, 59, right-left, 1, ink)

	// Today, or the last day with commits.
	label := "TODAY"
	if !r.ShownToday {
		if d, err := time.Parse(time.DateOnly, r.Shown.Date); err == nil {
			label = "LAST ACTIVE · " + strings.ToUpper(d.Format("Mon 2 Jan"))
		}
	}
	w(`<text x="%d" y="88" class="h">%s</text>`, left, label)
	add, del := "+"+comma(r.Shown.Add), "−"+comma(r.Shown.Del)
	size := fit(124, 300, func(s float64) float64 { return textWidth(add, s) + 12 + textWidth(del, s*0.42) })
	end := odometer(&b, "today", add, left, 208, size, 0.1)
	w(`<text x="%.1f" y="208" class="c" font-size="%.1f" fill="%s">%s</text>`, end+10, size*0.42, red, del)
	w(`<text x="%d" y="238" class="m">LINES · %s · %s</text>`, left, plural(r.Shown.Commits, "COMMIT"), plural(r.Shown.Files, "FILE"))
	if r.ShownTokens != nil {
		w(`<text x="%d" y="258" class="m">%s <tspan class="dim">TOKENS</tspan></text>`, left, comma64(r.ShownTokens.Total()))
	}
	rule(356, 76, 1, 190, hair)

	// Total.
	w(`<text x="380" y="88" class="h">TOTAL</text>`)
	loc := comma(r.LOC)
	odometer(&b, "total", loc, 380, 186, fit(100, 290, func(s float64) float64 { return textWidth(loc, s) }), 0.15)
	w(`<text x="380" y="218" class="m">LINES OF CODE<tspan class="dim">%s</tspan></text>`, esc(deltaText(r)))
	w(`<text x="380" y="238" class="m">%s · %s</text>`, plural(r.Repos, "REPO"), plural(r.Commits, "COMMIT"))
	if r.Tokens != nil {
		w(`<text x="380" y="258" class="m">%s <tspan class="dim">TOKENS SINCE %s</tspan></text>`, comma64(r.Tokens.Total()), dayLabel(r.TokensSince))
	}
	rule(680, 76, 1, 190, hair)

	// The last 30 days.
	w(`<text x="700" y="88" class="h">%d DAYS</text>`, windowDays)
	rule(700, 196, 100, 1, hair)
	most := 1
	var add30, del30, commits30, active int
	for _, d := range r.Days {
		most = max(most, d.Add+d.Del)
		add30 += d.Add
		del30 += d.Del
		commits30 += d.Commits
		if d.Commits > 0 {
			active++
		}
	}
	for i, d := range r.Days {
		h := 1.0
		if n := d.Add + d.Del; n > 0 {
			h = math.Max(3, 90*float64(n)/float64(most))
		}
		fill := ink
		if i == len(r.Days)-1 {
			fill = red
		}
		w(`<rect x="%.2f" y="%.1f" width="2.2" height="%.1f" fill="%s" class="grow" style="animation-delay:%.2fs"/>`,
			700+float64(i)*3.35, 196-h, h, fill, 0.4+float64(i)*0.02)
	}
	w(`<text x="700" y="214" class="m small">+%s <tspan fill="%s">−%s</tspan></text>`, comma(add30), red, comma(del30))
	w(`<text x="700" y="232" class="m small dim">%s</text>`, plural(commits30, "COMMIT"))
	w(`<text x="700" y="250" class="m small dim">%s</text>`, plural(active, "ACTIVE DAY"))

	rule(left, 280, right-left, 1, ink)

	// Languages by line count.
	w(`<text x="%d" y="308" class="h">LANGUAGES · LINES</text>`, left)
	for i, l := range r.Langs {
		y := 340 + 44*i
		name := esc(l.Name)
		if len(l.Others) > 0 {
			name += fmt.Sprintf(` <tspan class="m small dim">%s</tspan>`, esc(strings.ToUpper(otherNames(l.Others))))
		}
		w(`<text x="%d" y="%d" class="c name">%s</text>`, left, y, name)
		w(`<text x="300" y="%d" class="m end">%s</text>`, y, comma(l.Lines))
		w(`<text x="400" y="%d" class="c pct end">%s</text>`, y+2, percent(l.Frac))
		rule(left, y+10, 360, 3, hair)
		w(`<rect x="%d" y="%d" width="%.1f" height="3" fill="%s" class="fill" style="animation-delay:%.1fs"/>`,
			left, y+10, math.Max(2, 360*l.Frac), ink, 0.1*float64(i))
	}
	rule(420, 296, 1, 200, hair)

	// Tokens, all recorded days.
	if r.Tokens == nil {
		w(`<text x="440" y="308" class="h">TOKENS</text>`)
		w(`<text x="440" y="340" class="m dim">No token data yet</text>`)
	} else {
		renderTokens(&b, r)
	}

	// Footer.
	rule(left, 512, right-left, 1, ink)
	b.WriteString(aiMarks(left, 529))
	w(`<text x="%d" y="534" class="h end">GITHUB.COM/%s</text>`, right, esc(strings.ToUpper(r.Owner)))

	title := fmt.Sprintf("Daily code report: %s %s lines on %s; %s lines of code in total", add, del, r.Shown.Date, loc)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-labelledby="title">
<title id="title">%s</title>
<style>%s</style>
<rect width="%d" height="%d" fill="%s"/>
%s
</svg>
`, width, height, width, height, esc(title), stylesheet(), width, height, paper, b.String())
}

func renderTokens(b *strings.Builder, r *Report) {
	w := func(format string, a ...any) { fmt.Fprintf(b, format, a...) }
	t := *r.Tokens
	total := t.Total()
	w(`<text x="440" y="308" class="h">TOKENS · SINCE %s</text>`, dayLabel(r.TokensSince))
	sum := comma64(total)
	w(`<text x="440" y="352" class="c" font-size="%.1f">%s</text>`, fit(44, 360, func(s float64) float64 { return textWidth(sum, s) }), sum)

	kinds := []struct {
		name  string
		n     int64
		color string
	}{
		{"Cache read", t.CacheRead, gold},
		{"Cache write", t.CacheWrite, blue},
		{"Output", t.Output, red},
		{"Input", t.Input, ink},
	}
	// One stacked bar; a non-zero sliver stays at least 1.5px wide.
	x := 440.0
	for _, k := range kinds {
		if k.n == 0 || total == 0 {
			continue
		}
		seg := math.Max(1.5, 360*float64(k.n)/float64(total))
		seg = math.Min(seg, 800-x)
		w(`<rect x="%.1f" y="364" width="%.1f" height="8" fill="%s" class="fill"/>`, x, seg, k.color)
		x += seg
	}
	for i, k := range kinds {
		y := 400 + 28*i
		w(`<rect x="440" y="%d" width="12" height="3" fill="%s"/>`, y-6, k.color)
		w(`<text x="460" y="%d" class="c name small">%s</text>`, y, k.name)
		w(`<text x="720" y="%d" class="m end">%s</text>`, y, comma64(k.n))
		w(`<text x="800" y="%d" class="m end">%s</text>`, y, tokenShare(k.n, total))
	}
}

// odometer writes s in the big condensed face, each digit rolling up from 0
// when the image loads. It returns the x where the text ends.
func odometer(b *strings.Builder, id, s string, x, baseline, size, delay float64) float64 {
	end := x + textWidth(s, size)
	fmt.Fprintf(b, `<clipPath id="%s"><rect x="%.1f" y="%.1f" width="%.1f" height="%.1f"/></clipPath>`,
		id, x-4, baseline-0.76*size, end-x+8, 0.84*size)
	fmt.Fprintf(b, `<g clip-path="url(#%s)" class="c" font-size="%.1f">`, id, size)
	for _, r := range s {
		cx := x + advance(r)*size/2
		if r >= '0' && r <= '9' {
			d := int(r - '0')
			fmt.Fprintf(b, `<g class="roll r%d" style="animation-delay:%.2fs"><text class="mid">`, d, delay)
			for j := 0; j <= d; j++ {
				fmt.Fprintf(b, `<tspan x="%.1f" y="%.1f">%d</tspan>`, cx, baseline+float64(j)*size, j)
			}
			b.WriteString(`</text></g>`)
			delay += 0.08
		} else {
			fmt.Fprintf(b, `<text x="%.1f" y="%.1f" class="mid">%s</text>`, cx, baseline, esc(string(r)))
		}
		x += advance(r) * size
	}
	b.WriteString(`</g>`)
	return end
}

func stylesheet() string {
	var s strings.Builder
	fmt.Fprintf(&s, `@font-face{font-family:'Report Condensed';font-weight:600;src:url(data:font/woff2;base64,%s) format('woff2')}`,
		base64.StdEncoding.EncodeToString(condensedFont))
	s.WriteString(`text{fill:` + ink + `}`)
	s.WriteString(`.c{font-family:'Report Condensed','Barlow Condensed','Arial Narrow',sans-serif;font-weight:600;font-variant-numeric:tabular-nums}`)
	s.WriteString(`.h,.m{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}`)
	s.WriteString(`.h{font-size:10px;letter-spacing:2.2px;fill:#6A655C}`)
	s.WriteString(`.m{font-size:11px;letter-spacing:.6px}`)
	s.WriteString(`.small{font-size:10px}`)
	s.WriteString(`.dim{fill:#6A655C}`)
	s.WriteString(`.head{font-size:15px;letter-spacing:3px}`)
	s.WriteString(`.name{font-size:18px}.name.small{font-size:16px}`)
	s.WriteString(`.pct{font-size:26px}`)
	s.WriteString(`.mid{text-anchor:middle}.end{text-anchor:end}`)
	// Digits roll up by whole em steps (one line per digit).
	s.WriteString(`.roll{animation:1.6s cubic-bezier(.16,1,.3,1) both}`)
	for d := 1; d <= 9; d++ {
		fmt.Fprintf(&s, `.r%d{animation-name:r%d}@keyframes r%d{from{transform:translateY(0)}to{transform:translateY(-%dem)}}`, d, d, d, d)
	}
	s.WriteString(`.grow{transform-box:fill-box;transform-origin:50% 100%;animation:grow 1.2s cubic-bezier(.2,.8,.2,1) both}`)
	s.WriteString(`.fill{transform-box:fill-box;transform-origin:0 50%;animation:fill 1.4s cubic-bezier(.2,.8,.2,1) both}`)
	s.WriteString(`.live{animation:live 2s ease-in-out infinite}`)
	s.WriteString(`@keyframes grow{from{transform:scaleY(0)}to{transform:scaleY(1)}}`)
	s.WriteString(`@keyframes fill{from{transform:scaleX(0)}to{transform:scaleX(1)}}`)
	s.WriteString(`@keyframes live{0%,100%{opacity:1}50%{opacity:.25}}`)
	// Without motion every animation jumps to its end state.
	s.WriteString(`@media (prefers-reduced-motion:reduce){*{animation-duration:1ms!important;animation-delay:0s!important;animation-iteration-count:1!important}}`)
	return s.String()
}

// aiMarks draws the row of AI brand marks in ink, vertically centered on cy.
func aiMarks(x, cy int) string {
	const size, gap = 16, 14
	var b strings.Builder
	for _, l := range aiLogos {
		w, h, view := size, size, "0 0 24 24"
		if l.name == "Claude" {
			// The Claude Code mascot is 24x15 inside its square viewBox; crop
			// to it and give it a wider slot so it matches the others' weight.
			w, h, view = 22, 14, "0 5 24 15"
		}
		fmt.Fprintf(&b, `<svg x="%d" y="%d" width="%d" height="%d" viewBox="%s"><path d="%s" fill="%s" fill-rule="evenodd"/></svg>`,
			x, cy-h/2, w, h, view, l.path, ink)
		x += w + gap
	}
	return b.String()
}

// otherNames lists the languages folded into "Other", as many as fit beside
// the row's name.
func otherNames(names []string) string {
	const shown = 3
	if len(names) <= shown+1 {
		return strings.Join(names, " ")
	}
	return strings.Join(names[:shown], " ") + fmt.Sprintf(" +%d", len(names)-shown)
}

func deltaText(r *Report) string {
	switch {
	case !r.HasBaseline:
		return ""
	case r.LOCDelta > 0:
		return " · ↑ " + comma(r.LOCDelta) + " TODAY"
	case r.LOCDelta < 0:
		return " · ↓ " + comma(-r.LOCDelta) + " TODAY"
	default:
		return " · NO CHANGE TODAY"
	}
}

// dayLabel turns 2026-10-18 into "18 OCT".
func dayLabel(date string) string {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return esc(date)
	}
	return strings.ToUpper(d.Format("2 Jan"))
}

func percent(f float64) string {
	if f > 0 && f < 0.005 {
		return "<1%"
	}
	return strconv.Itoa(int(math.Round(f*100))) + "%"
}

func tokenShare(n, total int64) string {
	if total == 0 {
		return "0%"
	}
	p := 100 * float64(n) / float64(total)
	if n > 0 && p < 0.01 {
		return "&lt;0.01%"
	}
	return fmt.Sprintf("%.2f%%", p)
}

func comma(n int) string { return comma64(int64(n)) }

func comma64(n int64) string {
	if n < 0 {
		return "-" + comma64(-n)
	}
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return comma(n) + " " + word + "S"
}

func esc(s string) string { return html.EscapeString(s) }
