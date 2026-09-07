package main

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	reTrailingPrice = regexp.MustCompile(`^(.+?)(\d+(?:\.\d+)?)(?:文|元|块|￥|¥)?$`)
	reOnlyNumber    = regexp.MustCompile(`^\d+(?:\.\d+)?$`)
)

type extractedPair struct {
	Raw   string
	Name  string
	Price float64
}

// parseNamePrice extracts 名称/价格 from messy tokens.
// Rules:
//
//	软牡丹133      → 软牡丹 / 133
//	软牡丹123-133  → 软牡丹123 / 133
//	123..133      → 123 / 133
//
// 无价格（纯名称）返回 false。
func parseNamePrice(raw string) (extractedPair, bool) {
	s := normalizeToken(raw)
	if s == "" {
		return extractedPair{}, false
	}

	// 123..133 / 软牡丹..133
	if i := strings.LastIndex(s, ".."); i >= 0 {
		left := strings.TrimSpace(s[:i])
		right := strings.TrimSpace(s[i+2:])
		if left != "" {
			if p, ok := parseFloat(right); ok {
				return extractedPair{Raw: strings.TrimSpace(raw), Name: left, Price: p}, true
			}
		}
	}

	// 软牡丹123-133 / 软牡丹-133
	if i := strings.LastIndex(s, "-"); i >= 0 {
		left := strings.TrimSpace(s[:i])
		right := strings.TrimSpace(s[i+1:])
		if left != "" {
			if p, ok := parseFloat(right); ok {
				return extractedPair{Raw: strings.TrimSpace(raw), Name: left, Price: p}, true
			}
		}
	}

	// Pure number is not a name+price token
	if reOnlyNumber.MatchString(s) {
		return extractedPair{}, false
	}

	// 软牡丹133 → name=软牡丹, price=133
	m := reTrailingPrice.FindStringSubmatch(s)
	if m == nil {
		return extractedPair{}, false
	}
	name := strings.TrimSpace(m[1])
	if name == "" || !hasNameChar(name) {
		return extractedPair{}, false
	}
	p, ok := parseFloat(m[2])
	if !ok {
		return extractedPair{}, false
	}
	return extractedPair{Raw: strings.TrimSpace(raw), Name: name, Price: p}, true
}

func normalizeToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"．", ".",
		"。", ".",
		"—", "-",
		"–", "-",
		"－", "-",
		"〜", "-",
		"～", "-",
		"　", "",
		" ", "",
		"\t", "",
	)
	s = replacer.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if r >= '０' && r <= '９' {
			b.WriteRune(r - '０' + '0')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func hasNameChar(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.IsLetter(r) {
			return true
		}
	}
	for _, r := range s {
		if !unicode.IsDigit(r) && r != '.' {
			return true
		}
	}
	return false
}

func validPair(p extractedPair) bool {
	return strings.TrimSpace(p.Name) != "" && p.Raw != ""
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

func splitProducts(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool {
		switch r {
		case '\t', ' ', '　', ',', '，', ';', '；', '|', '/', '、':
			return true
		default:
			return false
		}
	})
}

func mostlySingleRunes(parts []string) bool {
	if len(parts) < 3 {
		return false
	}
	single := 0
	for _, p := range parts {
		if len([]rune(p)) == 1 {
			single++
		}
	}
	return single*2 >= len(parts)
}

// extractPairsFromLine parses one line that may contain many space-separated products.
func extractPairsFromLine(line string) []extractedPair {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	parts := splitProducts(line)
	if len(parts) == 0 {
		return nil
	}

	// 普 细 4 1 0 → 普细410
	if mostlySingleRunes(parts) {
		if p, ok := parseNamePrice(strings.Join(parts, "")); ok {
			return []extractedPair{p}
		}
	}

	var out []extractedPair
	add := func(p extractedPair) {
		if !validPair(p) {
			return
		}
		for i := range out {
			if out[i].Name == p.Name {
				out[i] = p
				return
			}
		}
		out = append(out, p)
	}

	for i := 0; i < len(parts); i++ {
		part := parts[i]
		nextIsBarePrice := false
		var nextPrice float64
		if i+1 < len(parts) {
			n := normalizeToken(parts[i+1])
			if reOnlyNumber.MatchString(n) {
				if p, ok := parseFloat(n); ok {
					nextIsBarePrice = true
					nextPrice = p
				}
			}
		}

		// 已粘在一起的「品名+价格」优先，避免把下一格裸价格误拼过来
		if p, ok := parseNamePrice(part); ok {
			add(p)
			continue
		}

		// 新红茶  290 / 金细  595
		if nextIsBarePrice {
			name := strings.TrimSpace(part)
			if name != "" && !isHeaderLabel(name) && !isSeriesHeader(name) && !reOnlyNumber.MatchString(normalizeToken(name)) {
				add(extractedPair{
					Raw:   name + " " + parts[i+1],
					Name:  name,
					Price: nextPrice,
				})
				i++
				continue
			}
		}
	}

	// 整行只有一个 token 时上面循环已处理；若全失败再试整行
	if len(out) == 0 && len(parts) == 1 {
		if p, ok := parseNamePrice(parts[0]); ok {
			add(p)
		}
	}
	return out
}

// extractPairsFromText pulls products from a cell.
// Supports:
//   - 一格一品：软牡丹133
//   - 一格多品 / 一行一条：换行分隔
//   - 同行多品：超多空格、逗号等分隔（数据表3 大单元格）
//   - 品名与价格之间有空格：新红茶  290
//
// 没有价格的条目会被过滤。
func extractPairsFromText(text string) []extractedPair {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var out []extractedPair
	add := func(p extractedPair) {
		if !validPair(p) {
			return
		}
		for i := range out {
			if out[i].Name == p.Name {
				out[i] = p
				return
			}
		}
		out = append(out, p)
	}

	for _, line := range splitLines(text) {
		for _, p := range extractPairsFromLine(line) {
			add(p)
		}
	}
	return out
}

// parseTwoColRow handles 名称列 + 价格列（含多组横向重复）.
// 两侧任一空、表头/系列分区、价格非数字 → 过滤。
// 有独立价格列时以该列为准，品名保持原样（不剥尾部数字）.
func parseTwoColRow(nameCell, priceCell string) (extractedPair, bool) {
	name := strings.TrimSpace(nameCell)
	priceRaw := strings.TrimSpace(priceCell)
	if name == "" || priceRaw == "" {
		return extractedPair{}, false
	}
	if isHeaderLabel(name) || isHeaderLabel(priceRaw) || isSeriesHeader(name) {
		return extractedPair{}, false
	}
	priceRaw = strings.TrimSuffix(strings.TrimSpace(priceRaw), "元")
	p, ok := parseFloat(normalizeToken(priceRaw))
	if !ok {
		return extractedPair{}, false
	}
	return extractedPair{
		Raw:   name + " " + priceRaw,
		Name:  name,
		Price: p,
	}, true
}

func isHeaderLabel(s string) bool {
	s = strings.TrimSpace(s)
	switch s {
	case "名称", "品名", "品名名称", "商品", "商品名",
		"价格", "单价", "报价", "售价",
		"出", "接", "出货", "接货":
		return true
	}
	return strings.Contains(s, "名称") && (strings.Contains(s, "价格") || strings.Contains(s, "售价"))
}

func isSeriesHeader(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// 金圣系列 / 公司外货系列 / 其它 .外 等分区标题
	if strings.HasSuffix(s, "系列") {
		return true
	}
	return false
}

// extractPairsFromRows collects pairs from sheet rows.
// Supports reference layouts:
//  1. 数据表.xlsx：名称 | 售价 | 名称 | 售价 …（空侧/系列分区跳过）
//  2. 数据表1.xlsx：每格「品名+价格」，可多行；无价格过滤
func extractPairsFromRows(rows [][]string) []extractedPair {
	byName := map[string]extractedPair{}
	order := make([]string, 0)

	add := func(p extractedPair) {
		if !validPair(p) {
			return
		}
		if _, exists := byName[p.Name]; !exists {
			order = append(order, p.Name)
		}
		byName[p.Name] = p
	}

	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		used := make([]bool, len(row))
		// 横向多组：名称 | 价格 | 名称 | 价格 …
		for i := 0; i+1 < len(row); i += 2 {
			if p, ok := parseTwoColRow(row[i], row[i+1]); ok {
				add(p)
				used[i] = true
				used[i+1] = true
			}
		}
		// 未配对单元格：一格一品 / 多品 / 多行
		for i, cell := range row {
			if used[i] {
				continue
			}
			for _, p := range extractPairsFromText(cell) {
				add(p)
			}
		}
	}

	out := make([]extractedPair, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}
