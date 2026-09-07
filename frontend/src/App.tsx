import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Dialogs, Events } from '@wailsio/runtime'
import { PriceService } from '../bindings/changeme'
import type {
  BatchApplyResult,
  BatchChangeRow,
  BatchPreviewItem,
  BatchPreviewResult,
  LogEntry,
  LogInfo,
  StatusInfo,
  UpdateResult,
} from '../bindings/changeme/models'
import { hasSeenTour, startTour } from './tour'
import { Modal } from './Modal'

const APP_VERSION = '1.0.0'
const DEBUG_CLICKS = 5
const PREVIEW_DIFF_ABS_DEFAULT = 50
/** 本地时间截止后静默锁定全部操作，不对用户提示 */
const USE_UNTIL = new Date(2026, 8, 10, 23, 0, 0).getTime()

function isPastUseUntil() {
  return Date.now() >= USE_UNTIL
}

function parsePreviewPrice(text: string): number | null {
  const n = Number(String(text ?? '').replace(/,/g, '').trim())
  return Number.isFinite(n) ? n : null
}

function previewPriceDiff(
  item: BatchPreviewItem,
  absThreshold: number,
): { delta: number; alert: boolean } | null {
  if (!item.matched) return null
  const old = parsePreviewPrice(item.oldPrice)
  if (old === null) return null
  const delta = item.newPrice - old
  const threshold = Number.isFinite(absThreshold) && absThreshold > 0 ? absThreshold : PREVIEW_DIFF_ABS_DEFAULT
  return { delta, alert: Math.abs(delta) >= threshold }
}

function formatDelta(delta: number) {
  const sign = delta > 0 ? '+' : ''
  return `${sign}${Number.isInteger(delta) ? delta : Number(delta.toFixed(2))}`
}

const emptyStatus: StatusInfo = {
  loaded: false,
  basePath: '',
  baseName: '',
  blockCount: 0,
  mappingCount: 0,
  dirty: false,
}

const emptyLog: LogInfo = { path: '', count: 0, lines: [] }
const xlsxFilter = [{ DisplayName: 'Excel', Pattern: '*.xlsx' }]

function fileNameOnly(path: string) {
  return path.split(/[/\\]/).pop() || path
}

function friendlyMessage(text: string, debug: boolean) {
  if (debug) return text
  return text
    .replace(/\/Users\/[^\s|；]+/g, (m) => fileNameOnly(m))
    .replace(/[A-Za-z]:\\[^\s|；]+/g, (m) => fileNameOnly(m))
}

function KindSeg({
  value,
  onChange,
  disabled,
}: {
  value: '出' | '接'
  onChange: (v: '出' | '接') => void
  disabled?: boolean
}) {
  return (
    <div className="seg" role="group" aria-label="出或接">
      <button
        type="button"
        className={value === '出' ? 'on' : ''}
        disabled={disabled}
        onClick={() => onChange('出')}
      >
        出
      </button>
      <button
        type="button"
        className={value === '接' ? 'on' : ''}
        disabled={disabled}
        onClick={() => onChange('接')}
      >
        接
      </button>
    </div>
  )
}

function levelLabel(level: string, debug: boolean) {
  if (debug) return level
  if (level === 'ok') return '成功'
  if (level === 'err') return '失败'
  return '提示'
}

function App() {
  const [status, setStatus] = useState<StatusInfo>(emptyStatus)
  const [name, setName] = useState('')
  const [kind, setKind] = useState<'出' | '接'>('出')
  const [batchKind, setBatchKind] = useState<'出' | '接'>('出')
  const [price, setPrice] = useState('')
  const [busy, setBusy] = useState(false)
  const [locked, setLocked] = useState(isPastUseUntil)
  const [flash, setFlash] = useState<{ type: 'ok' | 'err' | 'info'; text: string } | null>(null)
  const [logInfo, setLogInfo] = useState<LogInfo>(emptyLog)
  const [debug, setDebug] = useState(false)
  const [versionClicks, setVersionClicks] = useState(0)
  const [previewFile, setPreviewFile] = useState('')
  const [previewKind, setPreviewKind] = useState<'出' | '接'>('出')
  const [previewItems, setPreviewItems] = useState<BatchPreviewItem[]>([])
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewQuery, setPreviewQuery] = useState('')
  const [previewSearchText, setPreviewSearchText] = useState('')
  const previewSearchComposing = useRef(false)
  const [previewFilter, setPreviewFilter] = useState<'all' | 'matched' | 'miss' | 'diff'>('all')
  const [previewDiffAbs, setPreviewDiffAbs] = useState(String(PREVIEW_DIFF_ABS_DEFAULT))
  const [resultOpen, setResultOpen] = useState(false)
  const [resultRows, setResultRows] = useState<BatchChangeRow[]>([])
  const [resultTitle, setResultTitle] = useState('')
  const [mapTipOpen, setMapTipOpen] = useState(false)
  const [logDetail, setLogDetail] = useState<{ title: string; lines: string[] } | null>(null)
  const [logDetailOpen, setLogDetailOpen] = useState(false)
  const nameRef = useRef<HTMLInputElement>(null)
  const clickTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const tourBooted = useRef(false)

  const refreshLogs = useCallback(async () => {
    setLogInfo(await PriceService.GetLogs())
  }, [])

  const show = async (type: 'ok' | 'err' | 'info', text: string, lines: string[] = []) => {
    setFlash({ type, text })
    await PriceService.AppendLog(type, text, lines)
    await refreshLogs()
  }

  const onVersionClick = () => {
    if (locked) return
    clearTimeout(clickTimer.current)
    const next = versionClicks + 1
    if (next >= DEBUG_CLICKS) {
      setVersionClicks(0)
      setDebug((d) => !d)
      return
    }
    setVersionClicks(next)
    clickTimer.current = setTimeout(() => setVersionClicks(0), 1500)
  }

  useEffect(() => {
    setLocked(isPastUseUntil())
    const remain = USE_UNTIL - Date.now()
    if (remain <= 0) return
    const t = window.setTimeout(() => setLocked(true), remain)
    return () => window.clearTimeout(t)
  }, [])

  useEffect(() => {
    ;(async () => {
      try {
        setBusy(true)
        await refreshLogs()
        const s = await PriceService.Status()
        setStatus(s)
        if (s.loaded) {
          const tip =
            s.mappingCount > 0
              ? `已恢复基础表与映射（${s.mappingCount} 条）`
              : '已恢复基础表'
          setFlash({ type: 'info', text: tip })
        } else {
          setFlash({ type: 'info', text: '请先选择基础表' })
        }
      } catch (e) {
        await show('err', String(e))
      } finally {
        setBusy(false)
        if (!locked && !tourBooted.current && !hasSeenTour()) {
          tourBooted.current = true
          window.setTimeout(() => startTour(), 350)
        }
      }
    })()
  }, [])

  const pickBase = async () => {
    if (locked) return
    const path = await Dialogs.OpenFile({ Title: '选择基础表', Filters: xlsxFilter })
    if (!path || typeof path !== 'string') return
    await loadBase(path)
  }

  const loadBase = async (path: string) => {
    if (locked) return
    try {
      setBusy(true)
      const s = await PriceService.LoadBase(path)
      setStatus(s)
      await show('info', debug ? `已加载 ${s.baseName} · ${s.blockCount} 个区块` : '已加载基础表')
      nameRef.current?.focus()
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const pickMapping = async () => {
    if (locked) return
    const path = await Dialogs.OpenFile({ Title: '选择名称映射表', Filters: xlsxFilter })
    if (!path || typeof path !== 'string') return
    await loadMapping(path)
  }

  const loadMapping = async (path: string) => {
    if (locked) return
    try {
      setBusy(true)
      const s = await PriceService.LoadMapping(path)
      setStatus(s)
      await show('info', debug ? `映射已加载 · ${s.mappingCount} 条` : '映射表已加载')
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const clearMapping = async () => {
    if (locked) return
    setStatus(await PriceService.ClearMapping())
    await show('info', '已清除映射')
  }

  const doUpdate = async () => {
    if (locked) return
    const trimmed = name.trim()
    const p = Number(price)
    if (!trimmed) {
      await show('err', '请输入名称')
      nameRef.current?.focus()
      return
    }
    if (!Number.isFinite(p)) {
      await show('err', '请输入有效价格')
      return
    }
    try {
      setBusy(true)
      const res = (await PriceService.UpdatePrice(trimmed, kind, p)) as UpdateResult
      setStatus(await PriceService.Status())
      if (res.ok) {
        await show(
          'ok',
          `已更新「${res.resolvedName}」${res.kind}价为 ${res.price}`,
          res.updated ?? [],
        )
        setName('')
        nameRef.current?.focus()
      } else {
        await show('err', res.message)
      }
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const pickBatchFile = async () => {
    if (locked) return
    const path = await Dialogs.OpenFile({ Title: '选择其他价格表', Filters: xlsxFilter })
    if (!path || typeof path !== 'string') return
    await loadBatch(path)
  }

  const loadBatch = async (path: string) => {
    if (locked) return
    try {
      setBusy(true)
      const res = (await PriceService.PreviewPriceSheet(path, batchKind)) as BatchPreviewResult
      if (!res.ok || !(res.items?.length)) {
        setPreviewItems([])
        setPreviewFile('')
        setPreviewOpen(false)
        resetPreviewSearch()
        setPreviewFilter('all')
        setPreviewDiffAbs(String(PREVIEW_DIFF_ABS_DEFAULT))
        await show('err', res.message || '未识别到价格')
        return
      }
      setPreviewFile(res.fileName)
      setPreviewKind(res.kind as '出' | '接')
      setPreviewItems(res.items)
      resetPreviewSearch()
      setPreviewFilter('all')
      setPreviewDiffAbs(String(PREVIEW_DIFF_ABS_DEFAULT))
      setPreviewOpen(false)
      await show('info', `已加载 ${res.fileName}，${res.message}`)
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const clearPreview = async () => {
    if (locked) return
    await PriceService.ClearPendingBatch()
    setPreviewItems([])
    setPreviewFile('')
    setPreviewOpen(false)
    resetPreviewSearch()
    setPreviewFilter('all')
    setPreviewDiffAbs(String(PREVIEW_DIFF_ABS_DEFAULT))
    await show('info', '已清除文件列表')
  }

  const handlersRef = useRef({
    busy: false,
    locked: false,
    loadBase,
    loadMapping,
    loadBatch,
  })
  handlersRef.current = { busy, locked, loadBase, loadMapping, loadBatch }

  useEffect(() => {
    const off = Events.On('files-dropped', (ev) => {
      const data = (ev?.data ?? {}) as {
        path?: string
        role?: string
        error?: string
      }
      const h = handlersRef.current
      if (h.busy || h.locked) return
      if (data.error) {
        void show('err', data.error)
        return
      }
      const path = data.path
      if (!path) return
      const role = (data.role || '').replace(/^tour-/, '')
      if (role === 'base') void h.loadBase(path)
      else if (role === 'mapping') void h.loadMapping(path)
      else if (role === 'batch') void h.loadBatch(path)
      else void show('err', '请拖到基础表、名称映射或批量更新卡片上')
    })
    return () => off()
  }, [])

  const applyBatch = async () => {
    if (locked) return
    if (!previewItems.length) return
    try {
      setBusy(true)
      setPreviewOpen(false)
      const res = (await PriceService.ApplyPendingBatch()) as BatchApplyResult
      setStatus(await PriceService.Status())
      setPreviewItems([])
      setPreviewFile('')
      setResultRows(res.rows ?? [])
      setResultTitle(`${res.fileName || '批量更新'}（${res.kind}） ${res.message}`)
      setResultOpen(true)
      const details = (res.rows ?? [])
        .filter((r) => r.status === 'updated')
        .map(
          (r) =>
            `${r.resolvedName || r.name}：${r.oldPrice || '-'} → ${r.newPrice}`,
        )
      await show(res.ok ? 'ok' : 'err', `批量${res.kind}：${res.message}`, details)
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const undoOne = async (index: number) => {
    if (locked) return
    try {
      setBusy(true)
      const before = resultRows.find((r) => r.index === index)
      const res = (await PriceService.UndoBatchRow(index)) as BatchApplyResult
      setStatus(await PriceService.Status())
      setResultRows(res.rows ?? [])
      setResultTitle(`${res.fileName || '批量更新'}（${res.kind}） ${res.message}`)
      const detail = before
        ? [
            `${before.resolvedName || before.name}：${before.newPrice} → ${before.oldPrice || '-'}（撤回）`,
          ]
        : []
      await show('ok', res.message, detail)
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const undoAll = async () => {
    if (locked) return
    try {
      setBusy(true)
      const before = resultRows.filter((r) => r.canUndo && r.status === 'updated')
      const res = (await PriceService.UndoBatchAll()) as BatchApplyResult
      setStatus(await PriceService.Status())
      setResultRows(res.rows ?? [])
      setResultTitle(`${res.fileName || '批量更新'}（${res.kind}） ${res.message}`)
      const details = before.map(
        (r) => `${r.resolvedName || r.name}：${r.newPrice} → ${r.oldPrice || '-'}（撤回）`,
      )
      await show('ok', res.message, details)
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const doExport = async () => {
    if (locked) return
    const path = await Dialogs.SaveFile({
      Title: '导出更新后的基础表',
      Filename: '价格更新.xlsx',
      Filters: xlsxFilter,
    })
    if (!path) return
    try {
      setBusy(true)
      const saved = await PriceService.Export(path)
      setStatus(await PriceService.Status())
      await show('ok', debug ? `已导出 ${saved}` : `已导出 ${fileNameOnly(saved)}`)
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const clearLogs = async () => {
    if (locked) return
    try {
      setLogInfo(await PriceService.ClearLogs())
      setFlash({ type: 'info', text: '日志已清空' })
    } catch (e) {
      setFlash({ type: 'err', text: String(e) })
    }
  }

  const exportLogs = async () => {
    if (locked) return
    const stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')
    const path = await Dialogs.SaveFile({
      Title: '导出操作日志',
      Filename: `PriceMerge-日志-${stamp}.txt`,
      Filters: [
        { DisplayName: '文本日志', Pattern: '*.txt' },
        { DisplayName: 'Log', Pattern: '*.log' },
      ],
    })
    if (!path) return
    try {
      setBusy(true)
      const saved = await PriceService.ExportLogs(path)
      await show('ok', `日志已保存 ${fileNameOnly(saved)}`)
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const lines = logInfo.lines ?? []
  const matchedCount = previewItems.filter((i) => i.matched).length
  const diffAbsThreshold = (() => {
    const n = Number(previewDiffAbs)
    return Number.isFinite(n) && n > 0 ? n : PREVIEW_DIFF_ABS_DEFAULT
  })()
  const diffWarnCount = useMemo(
    () => previewItems.filter((i) => previewPriceDiff(i, diffAbsThreshold)?.alert).length,
    [previewItems, diffAbsThreshold],
  )
  const filteredPreviewItems = useMemo(() => {
    const q = previewQuery.trim().toLowerCase()
    return previewItems.filter((item) => {
      if (previewFilter === 'matched' && !item.matched) return false
      if (previewFilter === 'miss' && item.matched) return false
      if (previewFilter === 'diff' && !previewPriceDiff(item, diffAbsThreshold)?.alert) return false
      if (!q) return true
      const name = `${item.name} ${item.resolvedName}`.toLowerCase()
      return name.includes(q)
    })
  }, [previewItems, previewQuery, previewFilter, diffAbsThreshold])
  const undoableCount = resultRows.filter((r) => r.canUndo && r.status === 'updated').length

  const statusText = (status: string) => {
    if (status === 'updated') return '已更新'
    if (status === 'undone') return '已撤回'
    return '未匹配'
  }

  const resetPreviewSearch = () => {
    previewSearchComposing.current = false
    setPreviewQuery('')
    setPreviewSearchText('')
  }

  const onPreviewSearchChange = (value: string, composing: boolean) => {
    setPreviewSearchText(value)
    if (!composing) setPreviewQuery(value)
  }

  const openPreview = () => {
    resetPreviewSearch()
    setPreviewFilter('all')
    setPreviewOpen(true)
  }

  const removePreviewItem = async (item: BatchPreviewItem) => {
    if (locked || busy) return
    const index = previewItems.findIndex(
      (x) =>
        x.name === item.name &&
        x.newPrice === item.newPrice &&
        x.resolvedName === item.resolvedName,
    )
    if (index < 0) return
    try {
      setBusy(true)
      const res = (await PriceService.RemovePendingBatchItem(index)) as BatchPreviewResult
      const next = res.items ?? []
      setPreviewItems(next)
      if (next.length === 0) {
        setPreviewFile('')
        setPreviewOpen(false)
        resetPreviewSearch()
        setPreviewFilter('all')
        await show('info', '已清空预览列表')
      }
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className={`app ${debug ? 'debug' : ''}`}>
      <header className="header">
        <div className="brand">
          <img className="logo" src="./appicon.png" alt="" width={36} height={36} />
          <div className="brand-text">
            <div className="brand-line">
              <h1>PriceMerge</h1>
              <button type="button" className="version" disabled={locked} onClick={onVersionClick}>
                v{APP_VERSION}
              </button>
            </div>
            <p>出 / 接价格合并</p>
          </div>
        </div>
        <div className="header-actions">
          <button
            type="button"
            className="btn ghost"
            disabled={locked}
            onClick={() => startTour({ markSeen: true })}
          >
            教程
          </button>
          <button
            id="tour-export"
            className="btn export"
            disabled={locked || busy || !status.loaded}
            onClick={doExport}
          >
            导出 Excel
          </button>
        </div>
      </header>

      <div className="workspace">
        <section className="panel left">
          <div className="row-2">
            <article
              className="tile drop-tile"
              id="tour-base"
              {...(!locked
                ? { 'data-file-drop-target': true, 'data-drop-role': 'base' }
                : {})}
            >
              <div className="tile-body">
                <span className="label">基础表</span>
                <strong className="ellipsis">{status.loaded ? status.baseName : '未加载'}</strong>
                <span className="hint">
                  {status.loaded
                    ? status.dirty
                      ? '有未导出更改'
                      : '已就绪 · 可拖入更换'
                    : '点击选择或拖入 Excel'}
                  {debug && status.loaded ? ` · ${status.blockCount} 区块` : ''}
                </span>
              </div>
              <button className="btn ghost" disabled={locked || busy} onClick={pickBase}>
                {status.loaded ? '更换' : '选择'}
              </button>
            </article>

            <article
              className="tile drop-tile"
              id="tour-mapping"
              {...(!locked
                ? { 'data-file-drop-target': true, 'data-drop-role': 'mapping' }
                : {})}
            >
              <div className="tile-body">
                <span className="label">
                  名称映射
                  <span
                    className={`tips${mapTipOpen ? ' on' : ''}`}
                    tabIndex={0}
                    aria-label="格式说明"
                    onMouseEnter={() => setMapTipOpen(true)}
                    onMouseLeave={() => setMapTipOpen(false)}
                    onFocus={() => setMapTipOpen(true)}
                    onBlur={() => setMapTipOpen(false)}
                  >
                    ?
                    {mapTipOpen ? (
                      <span className="tips-pop" role="tooltip">
                        Excel 两列：A 其他表名称，B 基础表名称。
                        <br />
                        首行可为表头（含「名称」字样会自动跳过）。
                      </span>
                    ) : null}
                  </span>
                </span>
                <strong>{status.mappingCount > 0 ? `${status.mappingCount} 条` : '未使用'}</strong>
                <span className="hint">两列映射 · 可拖入</span>
              </div>
              <div className="tile-actions">
                <button
                  className="btn ghost"
                  disabled={locked || busy || status.mappingCount === 0}
                  onClick={clearMapping}
                >
                  清除
                </button>
                <button className="btn ghost" disabled={locked || busy} onClick={pickMapping}>
                  选择
                </button>
              </div>
            </article>
          </div>

          <article className="tile form-tile" id="tour-single">
            <div className="tile-title">
              <span className="label">单条更新</span>
            </div>
            <div className="form-row">
              <label className="field name-field">
                <span>名称</span>
                <input
                  ref={nameRef}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="品名"
                  autoComplete="off"
                  disabled={locked}
                  onKeyDown={(e) => e.key === 'Enter' && doUpdate()}
                />
              </label>
              <div className="field">
                <span>类型</span>
                <KindSeg value={kind} onChange={setKind} disabled={locked} />
              </div>
              <label className="field price-field">
                <span>价格</span>
                <input
                  value={price}
                  onChange={(e) => setPrice(e.target.value)}
                  placeholder="0"
                  inputMode="decimal"
                  autoComplete="off"
                  disabled={locked}
                  onKeyDown={(e) => e.key === 'Enter' && doUpdate()}
                />
              </label>
              <button
                className="btn primary"
                disabled={locked || busy || !status.loaded}
                onClick={doUpdate}
              >
                更新
              </button>
            </div>
          </article>

          <article
            className="tile form-tile drop-tile"
            id="tour-batch"
            {...(!locked
              ? { 'data-file-drop-target': true, 'data-drop-role': 'batch' }
              : {})}
          >
            <div className="tile-title">
              <span className="label">批量更新</span>
              <span className="hint">多列 / 空格分隔 / 品名+价</span>
            </div>
            <div className="form-row batch-row">
              <div className="field">
                <span>类型</span>
                <KindSeg value={batchKind} onChange={setBatchKind} disabled={locked} />
              </div>
              <button
                className="btn"
                disabled={locked || busy || !status.loaded}
                onClick={pickBatchFile}
              >
                上传文件
              </button>
            </div>

            {previewItems.length > 0 ? (
              <ul className="file-list">
                <li className="file-item">
                  <div className="file-meta">
                    <strong className="ellipsis" title={previewFile}>
                      {previewFile}
                    </strong>
                    <span className="hint">
                      {previewKind} · {previewItems.length} 条 · 可匹配 {matchedCount}
                    </span>
                  </div>
                  <div className="tile-actions">
                    <button
                      className="btn compact"
                      disabled={locked || busy}
                      onClick={openPreview}
                    >
                      查看
                    </button>
                    <button
                      className="btn primary compact"
                      disabled={locked || busy || matchedCount === 0}
                      onClick={applyBatch}
                    >
                      更新
                    </button>
                    <button
                      className="btn compact"
                      disabled={locked || busy}
                      onClick={clearPreview}
                    >
                      清除
                    </button>
                  </div>
                </li>
              </ul>
            ) : null}
          </article>

          <div className={`banner ${flash?.type ?? 'info'}`}>
            <i className="banner-dot" />
            <span className="ellipsis">{friendlyMessage(flash?.text ?? '就绪', debug)}</span>
          </div>
        </section>

        <section className="panel log" id="tour-log">
          <div className="log-top">
            <div className="tile-body">
              <span className="label">操作日志</span>
              <strong>{logInfo.count} 条</strong>
              {debug ? (
                <span className="hint ellipsis" title={logInfo.path}>
                  {logInfo.path || '日志文件'}
                </span>
              ) : null}
            </div>
            <div className="tile-actions">
              <button
                className="btn ghost"
                disabled={locked || busy || logInfo.count === 0}
                onClick={exportLogs}
              >
                导出
              </button>
              <button
                className="btn ghost"
                disabled={locked || busy || logInfo.count === 0}
                onClick={clearLogs}
              >
                清空
              </button>
            </div>
          </div>
          <div className="log-list">
            {lines.length === 0 ? (
              <div className="log-empty">暂无日志</div>
            ) : (
              lines.map((entry: LogEntry, i: number) => (
                <div className={`log-item ${entry.level}`} key={`${entry.time}-${i}`}>
                  <div className="log-meta">
                    <span className="log-time">{entry.time.slice(11) || '--:--:--'}</span>
                    <span className={`tag ${entry.level}`}>{levelLabel(entry.level, debug)}</span>
                    {entry.details?.length ? (
                      <button
                        type="button"
                        className="btn-text log-view"
                        disabled={locked}
                        onClick={() => {
                          setLogDetail({
                            title: friendlyMessage(entry.message, debug),
                            lines: entry.details ?? [],
                          })
                          setLogDetailOpen(true)
                        }}
                      >
                        查看
                      </button>
                    ) : null}
                  </div>
                  <p>{friendlyMessage(entry.message, debug)}</p>
                </div>
              ))
            )}
          </div>
        </section>
      </div>

      <Modal open={previewOpen} onClose={() => setPreviewOpen(false)}>
        <div className="modal-head">
          <div>
            <span className="label">价格预览</span>
            <strong>
              {previewFile}（{previewKind}）· {previewItems.length} 条 / 可匹配 {matchedCount}
              {diffWarnCount > 0 ? ` / 价差大 ${diffWarnCount}` : ''}
            </strong>
          </div>
          <button className="btn ghost" disabled={locked} onClick={() => setPreviewOpen(false)}>
            关闭
          </button>
        </div>
        <div className="modal-toolbar">
          <input
            className="modal-search"
            value={previewSearchText}
            onChange={(e) => {
              const composing =
                previewSearchComposing.current ||
                Boolean((e.nativeEvent as InputEvent).isComposing)
              onPreviewSearchChange(e.target.value, composing)
            }}
            onCompositionStart={() => {
              previewSearchComposing.current = true
            }}
            onCompositionEnd={(e) => {
              previewSearchComposing.current = false
              onPreviewSearchChange(e.currentTarget.value, false)
            }}
            placeholder="检索名称"
            autoComplete="off"
            disabled={locked}
          />
          <div className="seg filter-seg" role="group" aria-label="筛选">
            {(
              [
                ['all', '全部'],
                ['matched', '可匹配'],
                ['miss', '未匹配'],
                ['diff', '价差大'],
              ] as const
            ).map(([key, label]) => (
              <button
                key={key}
                type="button"
                className={previewFilter === key ? 'on' : ''}
                disabled={locked}
                onClick={() => setPreviewFilter(key)}
              >
                {label}
                {key === 'diff' && diffWarnCount > 0 ? ` ${diffWarnCount}` : ''}
              </button>
            ))}
          </div>
          <label className="diff-threshold">
            <span>≥</span>
            <input
              value={previewDiffAbs}
              onChange={(e) => setPreviewDiffAbs(e.target.value)}
              inputMode="decimal"
              placeholder={String(PREVIEW_DIFF_ABS_DEFAULT)}
              disabled={locked}
              aria-label="价差阈值"
            />
          </label>
          <span className="hint toolbar-count">
            显示 {filteredPreviewItems.length}/{previewItems.length}
          </span>
        </div>
        <div className="modal-body">
          <table className="data-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>原价</th>
                <th>新价</th>
                <th>差额</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {filteredPreviewItems.length === 0 ? (
                <tr>
                  <td colSpan={6} className="empty-row">
                    无匹配结果
                  </td>
                </tr>
              ) : (
                filteredPreviewItems.map((item) => {
                  const diff = previewPriceDiff(item, diffAbsThreshold)
                  const rowClass = [
                    item.matched ? '' : 'miss',
                    diff?.alert ? 'diff-warn' : '',
                  ]
                    .filter(Boolean)
                    .join(' ')
                  return (
                    <tr key={`${item.name}-${item.newPrice}-${item.resolvedName}`} className={rowClass}>
                      <td>
                        {item.resolvedName !== item.name
                          ? `${item.name}→${item.resolvedName}`
                          : item.name}
                      </td>
                      <td>{item.matched ? item.oldPrice || '-' : '-'}</td>
                      <td>{item.newPrice}</td>
                      <td className={diff?.alert ? 'diff-num' : ''}>
                        {diff ? formatDelta(diff.delta) : '-'}
                      </td>
                      <td>{item.matched ? (diff?.alert ? '价差大' : '可更新') : '未匹配'}</td>
                      <td>
                        <button
                          type="button"
                          className="btn-text danger"
                          disabled={locked || busy}
                          onClick={() => void removePreviewItem(item)}
                        >
                          删除
                        </button>
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>
      </Modal>

      <Modal open={resultOpen} onClose={() => setResultOpen(false)}>
        <div className="modal-head">
          <div>
            <span className="label">更新结果</span>
            <strong>{resultTitle}</strong>
          </div>
          <div className="tile-actions">
            <button
              className="btn ghost"
              disabled={locked || busy || undoableCount === 0}
              onClick={undoAll}
            >
              全部撤回
            </button>
            <button className="btn ghost" disabled={locked} onClick={() => setResultOpen(false)}>
              关闭
            </button>
          </div>
        </div>
        <div className="modal-body">
          <table className="data-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>老价格</th>
                <th>新价格</th>
                <th>结果</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {resultRows.map((row) => (
                <tr
                  key={`${row.index}-${row.name}`}
                  className={row.status === 'updated' ? '' : 'miss'}
                >
                  <td>{row.resolvedName || row.name}</td>
                  <td>{row.oldPrice || '-'}</td>
                  <td>{row.newPrice}</td>
                  <td>{statusText(row.status)}</td>
                  <td>
                    {row.canUndo && row.status === 'updated' ? (
                      <button
                        className="btn-text"
                        disabled={locked || busy}
                        onClick={() => undoOne(row.index)}
                      >
                        撤回
                      </button>
                    ) : (
                      <span className="hint">-</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Modal>

      <Modal open={logDetailOpen} onClose={() => setLogDetailOpen(false)}>
        <div className="modal-head">
          <div>
            <span className="label">更新明细</span>
            <strong>{logDetail?.title}</strong>
          </div>
          <button className="btn ghost" disabled={locked} onClick={() => setLogDetailOpen(false)}>
            关闭
          </button>
        </div>
        <div className="modal-body">
          <ul className="log-detail-list">
            {(logDetail?.lines ?? []).map((line, i) => (
              <li key={`${line}-${i}`}>{line}</li>
            ))}
          </ul>
        </div>
      </Modal>
    </main>
  )
}

export default App
