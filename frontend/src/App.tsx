import { useEffect, useRef, useState } from 'react'
import { Dialogs } from '@wailsio/runtime'
import { PriceService } from '../bindings/changeme'
import type { StatusInfo, UpdateResult } from '../bindings/changeme/models'

const emptyStatus: StatusInfo = {
  loaded: false,
  basePath: '',
  baseName: '',
  blockCount: 0,
  mappingCount: 0,
  dirty: false,
}

const xlsxFilter = [{ DisplayName: 'Excel', Pattern: '*.xlsx' }]

function App() {
  const [status, setStatus] = useState<StatusInfo>(emptyStatus)
  const [name, setName] = useState('')
  const [kind, setKind] = useState<'出' | '接'>('出')
  const [price, setPrice] = useState('')
  const [busy, setBusy] = useState(false)
  const [flash, setFlash] = useState<{ type: 'ok' | 'err' | 'info'; text: string } | null>(null)
  const [detail, setDetail] = useState<string[]>([])
  const nameRef = useRef<HTMLInputElement>(null)

  const show = (type: 'ok' | 'err' | 'info', text: string, lines: string[] = []) => {
    setFlash({ type, text })
    setDetail(lines)
  }

  useEffect(() => {
    ;(async () => {
      try {
        setBusy(true)
        const s = await PriceService.LoadDefaultTemplate()
        setStatus(s)
        show('info', `已加载 ${s.baseName} · ${s.blockCount} 个区块`)
      } catch (e) {
        show('err', String(e))
      } finally {
        setBusy(false)
        nameRef.current?.focus()
      }
    })()
  }, [])

  const pickBase = async () => {
    const path = await Dialogs.OpenFile({ Title: '选择基础表', Filters: xlsxFilter })
    if (!path || typeof path !== 'string') return
    try {
      setBusy(true)
      const s = await PriceService.LoadBase(path)
      setStatus(s)
      show('info', `已加载 ${s.baseName} · ${s.blockCount} 个区块`)
    } catch (e) {
      show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const pickMapping = async () => {
    const path = await Dialogs.OpenFile({ Title: '选择名称映射表', Filters: xlsxFilter })
    if (!path || typeof path !== 'string') return
    try {
      setBusy(true)
      const s = await PriceService.LoadMapping(path)
      setStatus(s)
      show('info', `映射已加载 · ${s.mappingCount} 条`)
    } catch (e) {
      show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  const clearMapping = async () => {
    setStatus(await PriceService.ClearMapping())
    show('info', '已清除映射')
  }

  const doUpdate = async () => {
    const trimmed = name.trim()
    const p = Number(price)
    if (!trimmed) {
      show('err', '请输入名称')
      nameRef.current?.focus()
      return
    }
    if (!Number.isFinite(p)) {
      show('err', '请输入有效价格')
      return
    }
    try {
      setBusy(true)
      const res = (await PriceService.UpdatePrice(trimmed, kind, p)) as UpdateResult
      setStatus(await PriceService.Status())
      if (res.ok) {
        show('ok', `${res.resolvedName} · ${res.kind} → ${res.price}`, res.updated ?? [])
        setName('')
        nameRef.current?.focus()
      } else {
        show('err', res.message)
      }
    } catch (e) {
      show('err', String(e))
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
      show('ok', `已导出 ${saved.split(/[/\\]/).pop()}`)
    } catch (e) {
      show('err', String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="shell">
      <header className="top">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true" />
          <div>
            <h1>PriceMerge</h1>
            <p>更新出 / 接 · 大琪有效价</p>
          </div>
        </div>
        <div className="top-actions">
          <button className="btn" disabled={busy || !status.loaded} onClick={doExport}>
            导出
          </button>
        </div>
      </header>

      <section className="files">
        <div className="file-card">
          <div className="file-meta">
            <span className="kicker">基础表</span>
            <strong className="truncate">{status.loaded ? status.baseName : '未加载'}</strong>
            <span className="sub">
              {status.loaded ? `${status.blockCount} 区块` : '内置模版'}
              {status.dirty ? ' · 未导出' : ''}
            </span>
          </div>
          <button className="btn quiet" disabled={busy} onClick={pickBase}>
            更换
          </button>
        </div>
        <div className="file-card">
          <div className="file-meta">
            <span className="kicker">名称映射</span>
            <strong>{status.mappingCount > 0 ? `${status.mappingCount} 条` : '未使用'}</strong>
            <span className="sub">其他名 → 基础名</span>
          </div>
          <div className="inline-btns">
            {status.mappingCount > 0 ? (
              <button className="btn quiet" disabled={busy} onClick={clearMapping}>
                清除
              </button>
            ) : null}
            <button className="btn quiet" disabled={busy} onClick={pickMapping}>
              选择
            </button>
          </div>
        </div>
      </section>

      <section className="editor">
        <div className="field grow">
          <label htmlFor="name">名称</label>
          <input
            ref={nameRef}
            id="name"
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="品名或映射名"
            autoComplete="off"
            onKeyDown={(e) => e.key === 'Enter' && doUpdate()}
          />
        </div>

        <div className="field">
          <span className="lbl">类型</span>
          <div className="seg" role="group" aria-label="出或接">
            <button
              type="button"
              className={kind === '出' ? 'on' : ''}
              onClick={() => setKind('出')}
            >
              出
            </button>
            <button
              type="button"
              className={kind === '接' ? 'on' : ''}
              onClick={() => setKind('接')}
            >
              接
            </button>
          </div>
        </div>

        <div className="field price">
          <label htmlFor="price">新价格</label>
          <input
            id="price"
            className="input"
            value={price}
            onChange={(e) => setPrice(e.target.value)}
            placeholder="0"
            inputMode="decimal"
            autoComplete="off"
            onKeyDown={(e) => e.key === 'Enter' && doUpdate()}
          />
        </div>

        <button className="btn primary update" disabled={busy || !status.loaded} onClick={doUpdate}>
          更新
        </button>
      </section>

      <footer className={`status ${flash?.type ?? 'info'}`}>
        <div className="status-main">
          <span className="dot" aria-hidden="true" />
          <span className="status-text">{flash?.text ?? '就绪'}</span>
        </div>
        {detail.length > 0 ? (
          <div className="status-detail truncate" title={detail.join('\n')}>
            {detail[0]}
            {detail.length > 1 ? ` 等 ${detail.length} 处` : ''}
          </div>
        ) : null}
      </footer>
    </main>
  )
}

export default App
