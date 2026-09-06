import { driver, type DriveStep } from 'driver.js'
import 'driver.js/dist/driver.css'
import './tour.css'

export const TOUR_SEEN_KEY = 'pricemerge.tour.seen'

const steps: DriveStep[] = [
  {
    element: '#tour-base',
    popover: {
      title: '① 选择基础表',
      description: '先打开你的 Excel 基础表（含「出 / 接」与「大琪」列）。可点「选择」或直接把 .xlsx 拖到此卡片。未选择前无法更新或导出。',
      side: 'bottom',
      align: 'start',
    },
  },
  {
    element: '#tour-mapping',
    popover: {
      title: '② 名称映射（可选）',
      description: '若价格表品名与基础表不一致，可上传或拖入两列映射：其他名 → 基础名。悬停「?」可查看格式。',
      side: 'bottom',
      align: 'start',
    },
  },
  {
    element: '#tour-single',
    popover: {
      title: '③ 单条更新',
      description: '输入品名、选择出/接、填写新价格后点「更新」。会尽量只改「差」，保留大琪公式。',
      side: 'bottom',
      align: 'start',
    },
  },
  {
    element: '#tour-batch',
    popover: {
      title: '④ 批量更新',
      description: '上传或拖入价格表后先出现文件列表：可「查看」预览，确认后再「更新」。更新结果支持单条/全部撤回。',
      side: 'top',
      align: 'start',
    },
  },
  {
    element: '#tour-export',
    popover: {
      title: '⑤ 导出',
      description: '有更改后点这里导出更新后的 Excel。记得先完成更新再导出。',
      side: 'bottom',
      align: 'end',
    },
  },
  {
    element: '#tour-log',
    popover: {
      title: '⑥ 操作日志',
      description: '这里记录每次操作结果，方便核对更新与导出情况。',
      side: 'left',
      align: 'start',
    },
  },
]

export function hasSeenTour(): boolean {
  try {
    return localStorage.getItem(TOUR_SEEN_KEY) === '1'
  } catch {
    return false
  }
}

export function markTourSeen(): void {
  try {
    localStorage.setItem(TOUR_SEEN_KEY, '1')
  } catch {
    /* ignore */
  }
}

export function startTour(options?: { markSeen?: boolean }): void {
  const markSeen = options?.markSeen !== false
  // Avoid overlapping tours (e.g. React StrictMode double-mount in dev)
  if (document.querySelector('.driver-overlay, .driver-active-element, .driver-popover')) {
    return
  }
  const tour = driver({
    showProgress: true,
    animate: true,
    allowClose: true,
    overlayColor: 'rgba(26, 35, 50, 0.55)',
    stagePadding: 8,
    stageRadius: 12,
    popoverClass: 'pm-tour-popover',
    nextBtnText: '下一步',
    prevBtnText: '上一步',
    doneBtnText: '开始使用',
    progressText: '{{current}} / {{total}}',
    steps,
    onDestroyed: () => {
      if (markSeen) markTourSeen()
    },
  })
  tour.drive()
}
