import request from '@/utils/http'

export interface MonitorJob {
  id: string
  name: string
  description: string
  enabled: boolean
  intervalSeconds: number
  lastRunAt: string
  lastStatus: string
  lastMessage: string
  nextRunAt: string
  running: boolean
}

export function fetchMonitorJobs() {
  return request.get<{ jobs: MonitorJob[] }>({ url: '/api/system/monitor/jobs' })
}

export function updateMonitorJob(id: string, enabled: boolean) {
  return request.put<{ jobs: MonitorJob[] }>({
    url: `/api/system/monitor/jobs/${encodeURIComponent(id)}`,
    data: { enabled },
    showSuccessMessage: true
  })
}

export function runMonitorJob(id: string) {
  return request.post<{ jobs: MonitorJob[] }>({
    url: `/api/system/monitor/jobs/${encodeURIComponent(id)}/run`,
    data: {},
    showSuccessMessage: true,
    timeout: 60000
  })
}
