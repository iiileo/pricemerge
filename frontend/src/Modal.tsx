import { useEffect, useState, type ReactNode, type MouseEvent } from 'react'

const EXIT_MS = 180

type ModalProps = {
  open: boolean
  onClose: () => void
  children: ReactNode
  labelledBy?: string
}

export function Modal({ open, onClose, children }: ModalProps) {
  const [mounted, setMounted] = useState(open)
  const [phase, setPhase] = useState<'enter' | 'show' | 'leave'>(open ? 'enter' : 'leave')

  useEffect(() => {
    if (open) {
      setMounted(true)
      setPhase('enter')
      const id = window.requestAnimationFrame(() => {
        window.requestAnimationFrame(() => setPhase('show'))
      })
      return () => window.cancelAnimationFrame(id)
    }
    if (!mounted) return
    setPhase('leave')
    const t = window.setTimeout(() => setMounted(false), EXIT_MS)
    return () => window.clearTimeout(t)
  }, [open, mounted])

  if (!mounted) return null

  const stop = (e: MouseEvent) => e.stopPropagation()

  return (
    <div
      className={`modal-backdrop is-${phase}`}
      onClick={onClose}
      role="presentation"
    >
      <div
        className={`modal is-${phase}`}
        onClick={stop}
        role="dialog"
        aria-modal="true"
      >
        {children}
      </div>
    </div>
  )
}
