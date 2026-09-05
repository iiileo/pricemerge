package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xuri/excelize/v2"
)

// PriceService updates 出/接 大琪 prices in the base workbook.
type PriceService struct {
	mu       sync.Mutex
	wb       *workbookIndex
	basePath string
	mapping  map[string]string // 其他表名称 -> 基础表名称
	dirty    bool
}

// StatusInfo is returned to the frontend about current loaded state.
type StatusInfo struct {
	Loaded       bool   `json:"loaded"`
	BasePath     string `json:"basePath"`
	BaseName     string `json:"baseName"`
	BlockCount   int    `json:"blockCount"`
	MappingCount int    `json:"mappingCount"`
	Dirty        bool   `json:"dirty"`
}

// UpdateResult describes one UpdatePrice call.
type UpdateResult struct {
	OK           bool     `json:"ok"`
	Message      string   `json:"message"`
	ResolvedName string   `json:"resolvedName"`
	Kind         string   `json:"kind"`
	Price        float64  `json:"price"`
	Updated      []string `json:"updated"`
}

// LoadBase opens a base workbook from disk.
func (s *PriceService) LoadBase(path string) (StatusInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path = strings.TrimSpace(path)
	if path == "" {
		return StatusInfo{}, fmt.Errorf("请选择基础表文件")
	}
	wb, err := openWorkbook(path)
	if err != nil {
		return StatusInfo{}, err
	}
	if s.wb != nil {
		_ = s.wb.close()
	}
	s.wb = wb
	s.basePath = path
	s.dirty = false
	return s.statusLocked(), nil
}

// LoadDefaultTemplate loads the embedded default base workbook.
func (s *PriceService) LoadDefaultTemplate() (StatusInfo, error) {
	data, err := defaultTemplateBytes()
	if err != nil {
		return StatusInfo{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	wb, err := openWorkbookBytes(data)
	if err != nil {
		return StatusInfo{}, err
	}
	if s.wb != nil {
		_ = s.wb.close()
	}
	s.wb = wb
	s.basePath = "3-16模版数据公式.xlsx"
	s.dirty = false
	return s.statusLocked(), nil
}

// LoadMapping loads a two-column name mapping sheet: 其他表名称 | 基础表名称.
func (s *PriceService) LoadMapping(path string) (StatusInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path = strings.TrimSpace(path)
	if path == "" {
		return StatusInfo{}, fmt.Errorf("请选择映射表文件")
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		return StatusInfo{}, fmt.Errorf("打开映射表失败: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return StatusInfo{}, fmt.Errorf("映射表没有工作表")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return StatusInfo{}, err
	}

	m := make(map[string]string)
	for i, row := range rows {
		if len(row) < 2 {
			continue
		}
		from := strings.TrimSpace(row[0])
		to := strings.TrimSpace(row[1])
		if from == "" || to == "" {
			continue
		}
		// Skip header-like first row
		if i == 0 && (strings.Contains(from, "名称") || strings.Contains(to, "名称")) {
			continue
		}
		m[from] = to
	}
	s.mapping = m
	return s.statusLocked(), nil
}

// ClearMapping clears the name mapping.
func (s *PriceService) ClearMapping() StatusInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mapping = nil
	return s.statusLocked()
}

// Status returns current load state.
func (s *PriceService) Status() StatusInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *PriceService) statusLocked() StatusInfo {
	info := StatusInfo{
		Loaded:       s.wb != nil,
		BasePath:     s.basePath,
		Dirty:        s.dirty,
		MappingCount: len(s.mapping),
	}
	if s.basePath != "" {
		info.BaseName = filepath.Base(s.basePath)
	}
	if s.wb != nil {
		info.BlockCount = len(s.wb.blocks)
	}
	return info
}

// UpdatePrice resolves name via mapping, finds matching 出/接 rows, writes 大琪 price.
func (s *PriceService) UpdatePrice(name string, kind string, price float64) (UpdateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.wb == nil {
		return UpdateResult{}, fmt.Errorf("请先加载基础表")
	}
	name = strings.TrimSpace(name)
	kind = strings.TrimSpace(kind)
	if name == "" {
		return UpdateResult{}, fmt.Errorf("请输入名称")
	}
	if kind != kindOut && kind != kindIn {
		return UpdateResult{}, fmt.Errorf("类型必须是「出」或「接」")
	}

	resolved := name
	if s.mapping != nil {
		if to, ok := s.mapping[name]; ok && to != "" {
			resolved = to
		}
	}

	hits, err := s.wb.findByName(kind, resolved)
	if err != nil {
		return UpdateResult{}, err
	}
	if len(hits) == 0 {
		msg := fmt.Sprintf("未找到：%s（%s）", resolved, kind)
		if resolved != name {
			msg = fmt.Sprintf("未找到：%s → %s（%s）", name, resolved, kind)
		}
		return UpdateResult{
			OK:           false,
			Message:      msg,
			ResolvedName: resolved,
			Kind:         kind,
			Price:        price,
			Updated:      nil,
		}, nil
	}

	updated := make([]string, 0, len(hits))
	for _, h := range hits {
		detail, err := s.wb.applyDaqiPrice(h, price)
		if err != nil {
			return UpdateResult{}, fmt.Errorf("写入失败: %w", err)
		}
		label := fmt.Sprintf("%s %s %s → %v（%s）", h.Kind, h.Region, h.OldPrice, price, detail)
		updated = append(updated, label)
	}
	s.dirty = true

	return UpdateResult{
		OK:           true,
		Message:      fmt.Sprintf("已更新 %d 处", len(updated)),
		ResolvedName: resolved,
		Kind:         kind,
		Price:        price,
		Updated:      updated,
	}, nil
}

// Export writes the current workbook to path.
func (s *PriceService) Export(path string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.wb == nil {
		return "", fmt.Errorf("请先加载基础表")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("请选择导出路径")
	}
	if !strings.HasSuffix(strings.ToLower(path), ".xlsx") {
		path += ".xlsx"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	if err := s.wb.saveAs(path); err != nil {
		return "", fmt.Errorf("导出失败: %w", err)
	}
	s.dirty = false
	return path, nil
}
