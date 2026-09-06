package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/xuri/excelize/v2"
)

// PriceService updates 出/接 大琪 prices in the base workbook.
type PriceService struct {
	mu        sync.Mutex
	wb        *workbookIndex
	basePath  string
	mapping   map[string]string // 其他表名称 -> 基础表名称
	dirty     bool
	log       *appLog
	pending   *pendingBatch
	lastApply *batchApplyState
}

type pendingBatch struct {
	Kind     string
	FileName string
	Items    []BatchPreviewItem
}

type batchApplyState struct {
	Kind     string
	FileName string
	Rows     []BatchChangeRow
}

func NewPriceService() *PriceService {
	return &PriceService{log: newAppLog()}
}

func (s *PriceService) ensureLog() *appLog {
	if s.log == nil {
		s.log = newAppLog()
	}
	return s.log
}

// AppendLog writes a log line to memory and the log file.
func (s *PriceService) AppendLog(level string, message string, details []string) LogEntry {
	return s.ensureLog().append(level, message, details)
}

// GetLogs returns recent log lines (newest first) and file path.
func (s *PriceService) GetLogs() LogInfo {
	return s.ensureLog().info()
}

// ClearLogs clears in-memory logs and empties the log file.
func (s *PriceService) ClearLogs() (LogInfo, error) {
	if err := s.ensureLog().clear(); err != nil {
		return LogInfo{}, fmt.Errorf("清空日志失败: %w", err)
	}
	return s.ensureLog().info(), nil
}

// ExportLogs saves current logs to the given path as a readable text file.
func (s *PriceService) ExportLogs(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("请选择保存路径")
	}
	if !strings.HasSuffix(strings.ToLower(path), ".txt") && !strings.HasSuffix(strings.ToLower(path), ".log") {
		path += ".txt"
	}
	if err := s.ensureLog().exportTo(path); err != nil {
		return "", fmt.Errorf("导出日志失败: %w", err)
	}
	return path, nil
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

// BatchImportResult is returned by ImportPriceSheet (legacy aggregate).
type BatchImportResult struct {
	OK          bool     `json:"ok"`
	Message     string   `json:"message"`
	Kind        string   `json:"kind"`
	FileName    string   `json:"fileName"`
	Extracted   int      `json:"extracted"`
	Updated     int      `json:"updated"`
	Missed      int      `json:"missed"`
	UpdatedList []string `json:"updatedList"`
	MissedList  []string `json:"missedList"`
	Samples     []string `json:"samples"`
}

// BatchPreviewItem is one extracted row shown before applying.
type BatchPreviewItem struct {
	Name         string  `json:"name"`
	ResolvedName string  `json:"resolvedName"`
	OldPrice     string  `json:"oldPrice"`
	NewPrice     float64 `json:"newPrice"`
	Matched      bool    `json:"matched"`
}

// BatchPreviewResult is returned by PreviewPriceSheet (no write yet).
type BatchPreviewResult struct {
	OK       bool               `json:"ok"`
	Message  string             `json:"message"`
	Kind     string             `json:"kind"`
	FileName string             `json:"fileName"`
	Items    []BatchPreviewItem `json:"items"`
}

// BatchChangeRow is one applied change for the result dialog.
type BatchChangeRow struct {
	Index        int     `json:"index"`
	Name         string  `json:"name"`
	ResolvedName string  `json:"resolvedName"`
	OldPrice     string  `json:"oldPrice"`
	NewPrice     float64 `json:"newPrice"`
	Status       string  `json:"status"` // updated | missed | undone
	CanUndo      bool    `json:"canUndo"`
}

// BatchApplyResult is returned after clicking 更新.
type BatchApplyResult struct {
	OK       bool             `json:"ok"`
	Message  string           `json:"message"`
	Kind     string           `json:"kind"`
	FileName string           `json:"fileName"`
	Updated  int              `json:"updated"`
	Missed   int              `json:"missed"`
	Rows     []BatchChangeRow `json:"rows"`
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
	s.pending = nil
	s.lastApply = nil
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
	return s.updatePriceLocked(name, kind, price)
}

func (s *PriceService) updatePriceLocked(name string, kind string, price float64) (UpdateResult, error) {
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
		label := fmt.Sprintf(
			"%s（%s·%s）%s→%v · %s",
			resolved, h.Kind, h.Region, emptyDash(h.OldPrice), price, detail,
		)
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

// PreviewPriceSheet extracts name/price pairs and looks up old prices; does not write.
func (s *PriceService) PreviewPriceSheet(path string, kind string) (BatchPreviewResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.wb == nil {
		return BatchPreviewResult{}, fmt.Errorf("请先加载基础表")
	}
	kind = strings.TrimSpace(kind)
	if kind != kindOut && kind != kindIn {
		return BatchPreviewResult{}, fmt.Errorf("类型必须是「出」或「接」")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return BatchPreviewResult{}, fmt.Errorf("请选择价格表")
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return BatchPreviewResult{}, fmt.Errorf("打开价格表失败: %w", err)
	}
	defer f.Close()

	byName := map[string]extractedPair{}
	order := make([]string, 0)
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, p := range extractPairsFromRows(rows) {
			if _, exists := byName[p.Name]; !exists {
				order = append(order, p.Name)
			}
			byName[p.Name] = p
		}
	}

	fileName := filepath.Base(path)
	if len(byName) == 0 {
		s.pending = nil
		return BatchPreviewResult{
			OK:       false,
			Message:  "未从价格表中提取到名称价格",
			Kind:     kind,
			FileName: fileName,
			Items:    nil,
		}, nil
	}

	items := make([]BatchPreviewItem, 0, len(order))
	matched := 0
	for _, name := range order {
		p := byName[name]
		resolved := name
		if s.mapping != nil {
			if to, ok := s.mapping[name]; ok && to != "" {
				resolved = to
			}
		}
		old := ""
		okMatch := false
		hits, err := s.wb.findByName(kind, resolved)
		if err == nil && len(hits) > 0 {
			okMatch = true
			matched++
			old = hits[0].OldPrice
		}
		items = append(items, BatchPreviewItem{
			Name:         name,
			ResolvedName: resolved,
			OldPrice:     old,
			NewPrice:     p.Price,
			Matched:      okMatch,
		})
	}

	s.pending = &pendingBatch{Kind: kind, FileName: fileName, Items: items}
	return BatchPreviewResult{
		OK:       true,
		Message:  fmt.Sprintf("共 %d 条，可匹配 %d 条", len(items), matched),
		Kind:     kind,
		FileName: fileName,
		Items:    items,
	}, nil
}

// ClearPendingBatch clears the preview list.
func (s *PriceService) ClearPendingBatch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = nil
}

// ApplyPendingBatch writes pending preview items into the base workbook.
func (s *PriceService) ApplyPendingBatch() (BatchApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.wb == nil {
		return BatchApplyResult{}, fmt.Errorf("请先加载基础表")
	}
	if s.pending == nil || len(s.pending.Items) == 0 {
		return BatchApplyResult{}, fmt.Errorf("请先上传价格表")
	}

	kind := s.pending.Kind
	fileName := s.pending.FileName
	rows := make([]BatchChangeRow, 0, len(s.pending.Items))
	updated := 0
	missed := 0

	for i, item := range s.pending.Items {
		res, err := s.updatePriceLocked(item.Name, kind, item.NewPrice)
		if err != nil {
			return BatchApplyResult{}, err
		}
		row := BatchChangeRow{
			Index:        i,
			Name:         item.Name,
			ResolvedName: res.ResolvedName,
			OldPrice:     item.OldPrice,
			NewPrice:     item.NewPrice,
			Status:       "missed",
			CanUndo:      false,
		}
		if res.OK {
			updated++
			row.Status = "updated"
			_, ok := parsePriceText(item.OldPrice)
			row.CanUndo = ok
		} else {
			missed++
		}
		rows = append(rows, row)
	}

	s.pending = nil
	s.lastApply = &batchApplyState{Kind: kind, FileName: fileName, Rows: rows}
	msg := fmt.Sprintf("更新 %d · 未匹配 %d", updated, missed)
	return BatchApplyResult{
		OK:       updated > 0,
		Message:  msg,
		Kind:     kind,
		FileName: fileName,
		Updated:  updated,
		Missed:   missed,
		Rows:     cloneBatchRows(rows),
	}, nil
}

// UndoBatchRow restores one updated row to its old price.
func (s *PriceService) UndoBatchRow(index int) (BatchApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastApply == nil {
		return BatchApplyResult{}, fmt.Errorf("没有可撤回的更新")
	}
	if index < 0 || index >= len(s.lastApply.Rows) {
		return BatchApplyResult{}, fmt.Errorf("无效的撤回项")
	}
	row := &s.lastApply.Rows[index]
	if row.Status != "updated" || !row.CanUndo {
		return BatchApplyResult{}, fmt.Errorf("该项不可撤回")
	}
	old, ok := parsePriceText(row.OldPrice)
	if !ok {
		return BatchApplyResult{}, fmt.Errorf("无法解析老价格：%s", row.OldPrice)
	}
	res, err := s.updatePriceLocked(row.Name, s.lastApply.Kind, old)
	if err != nil {
		return BatchApplyResult{}, err
	}
	if !res.OK {
		return BatchApplyResult{}, fmt.Errorf("%s", res.Message)
	}
	row.Status = "undone"
	row.CanUndo = false
	return s.batchResultFromLastLocked("已撤回 1 条"), nil
}

// UndoBatchAll restores all remaining updated rows to their old prices.
func (s *PriceService) UndoBatchAll() (BatchApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastApply == nil {
		return BatchApplyResult{}, fmt.Errorf("没有可撤回的更新")
	}
	n := 0
	for i := range s.lastApply.Rows {
		row := &s.lastApply.Rows[i]
		if row.Status != "updated" || !row.CanUndo {
			continue
		}
		old, ok := parsePriceText(row.OldPrice)
		if !ok {
			continue
		}
		res, err := s.updatePriceLocked(row.Name, s.lastApply.Kind, old)
		if err != nil {
			return BatchApplyResult{}, err
		}
		if !res.OK {
			return BatchApplyResult{}, fmt.Errorf("%s", res.Message)
		}
		row.Status = "undone"
		row.CanUndo = false
		n++
	}
	if n == 0 {
		return BatchApplyResult{}, fmt.Errorf("没有可撤回的更新")
	}
	return s.batchResultFromLastLocked(fmt.Sprintf("已撤回 %d 条", n)), nil
}

func (s *PriceService) batchResultFromLastLocked(msg string) BatchApplyResult {
	st := s.lastApply
	updated, missed, undone := 0, 0, 0
	for _, r := range st.Rows {
		switch r.Status {
		case "updated":
			updated++
		case "undone":
			undone++
		default:
			missed++
		}
	}
	full := msg
	if undone > 0 {
		full = fmt.Sprintf("%s · 仍更新 %d · 已撤回 %d · 未匹配 %d", msg, updated, undone, missed)
	}
	return BatchApplyResult{
		OK:       true,
		Message:  full,
		Kind:     st.Kind,
		FileName: st.FileName,
		Updated:  updated,
		Missed:   missed,
		Rows:     cloneBatchRows(st.Rows),
	}
}

func cloneBatchRows(rows []BatchChangeRow) []BatchChangeRow {
	out := make([]BatchChangeRow, len(rows))
	copy(out, rows)
	return out
}

func parsePriceText(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func emptyDash(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	return s
}

// ImportPriceSheet is kept for tests: preview + apply in one call.
func (s *PriceService) ImportPriceSheet(path string, kind string) (BatchImportResult, error) {
	prev, err := s.PreviewPriceSheet(path, kind)
	if err != nil {
		return BatchImportResult{}, err
	}
	if !prev.OK || len(prev.Items) == 0 {
		return BatchImportResult{
			OK:       false,
			Message:  prev.Message,
			Kind:     kind,
			FileName: prev.FileName,
		}, nil
	}
	applied, err := s.ApplyPendingBatch()
	if err != nil {
		return BatchImportResult{}, err
	}
	updatedList := make([]string, 0)
	missedList := make([]string, 0)
	for _, r := range applied.Rows {
		line := fmt.Sprintf("%s: %s → %v", r.ResolvedName, r.OldPrice, r.NewPrice)
		if r.Status == "updated" {
			updatedList = append(updatedList, line)
		} else {
			missedList = append(missedList, line)
		}
	}
	return BatchImportResult{
		OK:          applied.OK,
		Message:     applied.Message,
		Kind:        kind,
		FileName:    applied.FileName,
		Extracted:   len(prev.Items),
		Updated:     applied.Updated,
		Missed:      applied.Missed,
		UpdatedList: updatedList,
		MissedList:  missedList,
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
