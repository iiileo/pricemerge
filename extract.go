package main

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	reTrailingPrice = regexp.MustCompile(`^(.+?)(\d+(?:\.\d+)?)$`)
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

// extractPairsFromText pulls products from a cell.
// Supports:
//   - 一格一品：软牡丹133
//   - 一格多品 / 一行一条：换行分隔
//   - 同行多品：空格、逗号等分隔
//
// 没有价格的条目会被过滤。
func extractPairsFromText(text string) []extractedPair {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var out []extractedPair
	seen := map[string]bool{}
	add := func(p extractedPair) {
		if !validPair(p) {
			return
		}
		if seen[p.Name] {
			// 同单元格内后写覆盖前写：先删旧再追加顺序末尾由上层 byName 处理
		}
		seen[p.Name] = true
		// replace existing same name in out
		for i := range out {
			if out[i].Name == p.Name {
				out[i] = p
				return
			}
		}
		out = append(out, p)
	}

	lines := splitLines(text)
	// 单行
	if len(lines) == 1 {
		line := strings.TrimSpace(lines[0])
		parts := splitProducts(line)
		// 同行多品：先按分隔符拆开，避免去空格后粘成一个错误品名
		if len(parts) > 1 {
			for _, part := range parts {
				if p, ok := parseNamePrice(part); ok {
					add(p)
				}
			}
			return out
		}
		if p, ok := parseNamePrice(line); ok {
			add(p)
		}
		return out
	}

	// 多行：优先一行一条；单行解析失败再拆分
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if p, ok := parseNamePrice(line); ok {
			add(p)
			continue
		}
		for _, part := range splitProducts(line) {
			if p, ok := parseNamePrice(part); ok {
				add(p)
			}
		}
	}
	return out
}

// parseTwoColRow handles 第一列名称 + 第二列价格.
// 第二列无价格则返回 false（过滤）。
func parseTwoColRow(nameCell, priceCell string) (extractedPair, bool) {
	name := strings.TrimSpace(nameCell)
	priceRaw := strings.TrimSpace(priceCell)
	if name == "" || priceRaw == "" {
		return extractedPair{}, false
	}
	if isHeaderLabel(name) || isHeaderLabel(priceRaw) {
		return extractedPair{}, false
	}
	priceRaw = strings.TrimSuffix(strings.TrimSpace(priceRaw), "元")
	p, ok := parseFloat(normalizeToken(priceRaw))
	if !ok {
		return extractedPair{}, false
	}
	if embedded, ok := parseNamePrice(name); ok {
		return embedded, true
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
	case "名称", "品名", "品名名称", "商品", "商品名", "价格", "单价", "报价", "出", "接", "出货", "接货":
		return true
	}
	return strings.Contains(s, "名称") && strings.Contains(s, "价格")
}

// extractPairsFromRows collects pairs from sheet rows:
// 1) 两列：名称 | 价格（无价格行跳过）
// 2) 每格一品 / 一格多品 / 一行一条
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
		// 标准两列：名称 | 价格
		if len(row) >= 2 {
			if p, ok := parseTwoColRow(row[0], row[1]); ok {
				add(p)
				continue
			}
		}
		// 其余：逐格提取（一格一品 / 多品 / 多行）
		for _, cell := range row {
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
