import { useCallback, useEffect, useRef, useState } from 'react'
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
}: {
  value: '出' | '接'
  onChange: (v: '出' | '接') => void
}) {
  return (
    <div className="seg" role="group" aria-label="出或接">
      <button type="button" className={value === '出' ? 'on' : ''} onClick={() => onChange('出')}>
        出
      </button>
      <button type="button" className={value === '接' ? 'on' : ''} onClick={() => onChange('接')}>
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
  const [flash, setFlash] = useState<{ type: 'ok' | 'err' | 'info'; text: string } | null>(null)
  const [logInfo, setLogInfo] = useState<LogInfo>(emptyLog)
  const [debug, setDebug] = useState(false)
  const [versionClicks, setVersionClicks] = useState(0)
  const [previewFile, setPreviewFile] = useState('')
  const [previewKind, setPreviewKind] = useState<'出' | '接'>('出')
  const [previewItems, setPreviewItems] = useState<BatchPreviewItem[]>([])
  const [previewOpen, setPreviewOpen] = useState(false)
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
    ;(async () => {
      try {
        setBusy(true)
        await refreshLogs()
        setStatus(await PriceService.Status())
        setFlash({ type: 'info', text: '请先选择基础表' })
      } catch (e) {
        await show('err', String(e))
      } finally {
        setBusy(false)
        if (!tourBooted.current && !hasSeenTour()) {
          tourBooted.current = true
          window.setTimeout(() => startTour(), 350)
        }
      }
    })()
  }, [])

  const pickBase = async () => {
    const path = await Dialogs.OpenFile({ Title: '选择基础表', Filters: xlsxFilter })
    if (!path || typeof path !== 'string') return
    await loadBase(path)
  }

  const loadBase = async (path: string) => {
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
    const path = await Dialogs.OpenFile({ Title: '选择名称映射表', Filters: xlsxFilter })
    if (!path || typeof path !== 'string') return
    await loadMapping(path)
  }

  const loadMapping = async (path: string) => {
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
    setStatus(await PriceService.ClearMapping())
    await show('info', '已清除映射')
  }

  const doUpdate = async () => {
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
    const path = await Dialogs.OpenFile({ Title: '选择其他价格表', Filters: xlsxFilter })
    if (!path || typeof path !== 'string') return
    await loadBatch(path)
  }

  const loadBatch = async (path: string) => {
    try {
      setBusy(true)
      const res = (await PriceService.PreviewPriceSheet(path, batchKind)) as BatchPreviewResult
      if (!res.ok || !(res.items?.length)) {
        setPreviewItems([])
        setPreviewFile('')
        setPreviewOpen(false)
        await show('err', res.message || '未识别到价格')
        return
      }
      setPreviewFile(res.fileName)
      setPreviewKind(res.kind as '出' | '接')
      setPreviewItems(res.items)
      setPreviewOpen(false)
      await show('info', `已加载 ${res.fileName}，${res.message}`)
    } catch (e) {
      await show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const clearPreview = async () => {
    await PriceService.ClearPendingBatch()
    setPreviewItems([])
    setPreviewFile('')
    setPreviewOpen(false)
    await show('info', '已清除文件列表')
  }

  const handlersRef = useRef({
    busy: false,
    loadBase,
    loadMapping,
    loadBatch,
  })
  handlersRef.current = { busy, loadBase, loadMapping, loadBatch }

  useEffect(() => {
    const off = Events.On('files-dropped', (ev) => {
      const data = (ev?.data ?? {}) as {
        path?: string
        role?: string
        error?: string
      }
      const h = handlersRef.current
      if (h.busy) return
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
    try {
      setLogInfo(await PriceService.ClearLogs())
      setFlash({ type: 'info', text: '日志已清空' })
    } catch (e) {
      setFlash({ type: 'err', text: String(e) })
    }
  }

  const exportLogs = async () => {
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
  const undoableCount = resultRows.filter((r) => r.canUndo && r.status === 'updated').length

  const statusText = (status: string) => {
    if (status === 'updated') return '已更新'
    if (status === 'undone') return '已撤回'
    return '未匹配'
  }

  return (
    <main className={`app ${debug ? 'debug' : ''}`}>
      <header className="header">
        <div className="brand">
          <img className="logo" src="./appicon.png" alt="" width={36} height={36} />
          <div className="brand-text">
            <div className="brand-line">
              <h1>PriceMerge</h1>
              <button type="button" className="version" onClick={onVersionClick}>
                v{APP_VERSION}
              </button>
            </div>
            <p>出 / 接价格合并</p>
          </div>
        </div>
        <div className="header-actions">
          <button type="button" className="btn ghost" onClick={() => startTour({ markSeen: true })}>
            教程
          </button>
          <button
            id="tour-export"
            className="btn export"
            disabled={busy || !status.loaded}
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
              data-file-drop-target
              data-drop-role="base"
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
              <button className="btn ghost" disabled={busy} onClick={pickBase}>
                {status.loaded ? '更换' : '选择'}
              </button>
            </article>

            <article
              className="tile drop-tile"
              id="tour-mapping"
              data-file-drop-target
              data-drop-role="mapping"
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
                <span className="hint">两列：其他名 | 基础名 · 可拖入</span>
              </div>
              <div className="tile-actions">
                {status.mappingCount > 0 ? (
                  <button className="btn ghost" disabled={busy} onClick={clearMapping}>
                    清除
                  </button>
                ) : null}
                <button className="btn ghost" disabled={busy} onClick={pickMapping}>
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
                  onKeyDown={(e) => e.key === 'Enter' && doUpdate()}
                />
              </label>
              <div className="field">
                <span>类型</span>
                <KindSeg value={kind} onChange={setKind} />
              </div>
              <label className="field price-field">
                <span>价格</span>
                <input
                  value={price}
                  onChange={(e) => setPrice(e.target.value)}
                  placeholder="0"
                  inputMode="decimal"
                  autoComplete="off"
                  onKeyDown={(e) => e.key === 'Enter' && doUpdate()}
                />
              </label>
              <button className="btn primary" disabled={busy || !status.loaded} onClick={doUpdate}>
                更新
              </button>
            </div>
          </article>

          <article
            className="tile form-tile drop-tile"
            id="tour-batch"
            data-file-drop-target
            data-drop-role="batch"
          >
            <div className="tile-title">
              <span className="label">批量更新</span>
              <span className="hint">上传或拖入价格表，查看确认后再更新</span>
            </div>
            <div className="form-row batch-row">
              <div className="field">
                <span>类型</span>
                <KindSeg value={batchKind} onChange={setBatchKind} />
              </div>
              <button className="btn" disabled={busy || !status.loaded} onClick={pickBatchFile}>
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
                    <button className="btn compact" disabled={busy} onClick={() => setPreviewOpen(true)}>
                      查看
                    </button>
                    <button
                      className="btn primary compact"
                      disabled={busy || matchedCount === 0}
                      onClick={applyBatch}
                    >
                      更新
                    </button>
                    <button className="btn compact" disabled={busy} onClick={clearPreview}>
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
              <button className="btn ghost" disabled={busy || logInfo.count === 0} onClick={exportLogs}>
                导出
              </button>
              <button className="btn ghost" disabled={busy || logInfo.count === 0} onClick={clearLogs}>
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
            </strong>
          </div>
          <button className="btn ghost" onClick={() => setPreviewOpen(false)}>
            关闭
          </button>
        </div>
        <div className="modal-body">
          <table className="data-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>原价</th>
                <th>新价</th>
                <th>状态</th>
              </tr>
            </thead>
            <tbody>
              {previewItems.map((item) => (
                <tr key={`${item.name}-${item.newPrice}`} className={item.matched ? '' : 'miss'}>
                  <td>
                    {item.resolvedName !== item.name
                      ? `${item.name}→${item.resolvedName}`
                      : item.name}
                  </td>
                  <td>{item.matched ? item.oldPrice || '-' : '-'}</td>
                  <td>{item.newPrice}</td>
                  <td>{item.matched ? '可更新' : '未匹配'}</td>
                </tr>
              ))}
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
            <button className="btn ghost" disabled={busy || undoableCount === 0} onClick={undoAll}>
              全部撤回
            </button>
            <button className="btn ghost" onClick={() => setResultOpen(false)}>
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
                        disabled={busy}
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
          <button className="btn ghost" onClick={() => setLogDetailOpen(false)}>
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
